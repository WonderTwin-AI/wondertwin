//go:build ignore

// gen_response_schema writes response_schema_gen.go: for each Stripe object
// the emulator serves, the properties Stripe returns that a Go struct with
// omitempty leaves out, and what Stripe returns for them when they are unset.
//
// It reads the pinned dahlia GA spec, which is not in this repository. Restore
// it from the research corpus (wondertwin-docs research/twins/stripe,
// reference openapi-v1-2026-08-26.dahlia) and run:
//
//	go run gen_response_schema.go -spec /path/to/stripe-v1-2026-08-26.dahlia.json
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"
)

// served are the object values the emulator returns. A new resource type adds
// its object value here and regenerates.
var served = []string{
	"account", "account_link", "balance", "balance_transaction", "bank_account",
	"billing_portal.session", "charge", "checkout.session", "coupon", "credit_note",
	"customer", "discount", "dispute", "event", "fee_refund", "file", "file_link",
	"invoice", "invoiceitem", "item", "line_item", "payment_intent", "payment_link",
	"payment_method", "payout", "person", "price", "product", "promotion_code",
	"quote", "refund", "review", "setup_intent", "shipping_rate", "source",
	"subscription", "subscription_item", "tax_id", "tax_rate", "token", "topup",
	"transfer", "transfer_reversal", "webhook_endpoint",
}

// includable are properties Stripe documents as "includable (not returned by
// default; request it with the expand request parameter)". The spec does not
// mark them; these come from the captured API reference pages
// (docs-2026-09-28-pages, api/customers/object). They are never filled.
var includable = map[string][]string{
	"customer": {"cash_balance", "invoice_credit_balance", "sources", "subscriptions", "tax", "tax_ids"},
}

const specID = "openapi-v1-2026-08-26.dahlia"

type schema struct {
	Ref                  string             `json:"$ref"`
	Type                 string             `json:"type"`
	Nullable             bool               `json:"nullable"`
	Enum                 []any              `json:"enum"`
	Properties           map[string]*schema `json:"properties"`
	Items                *schema            `json:"items"`
	AnyOf                []*schema          `json:"anyOf"`
	AdditionalProperties json.RawMessage    `json:"additionalProperties"`
	ResourceID           string             `json:"x-resourceId"`
}

type spec struct {
	Components struct {
		Schemas map[string]*schema `json:"schemas"`
	} `json:"components"`
}

var schemas map[string]*schema

func refName(s *schema) string { return strings.TrimPrefix(s.Ref, "#/components/schemas/") }

// isResource reports whether a schema is a top-level object with its own
// "object" value. Those are found at runtime from that value.
func isResource(name string) bool {
	s := schemas[name]
	return s != nil && s.Properties["object"] != nil && len(s.Properties["object"].Enum) == 1 &&
		s.Properties["object"].Enum[0] != "list"
}

// embedded returns the single embedded sub-object schema a property holds,
// or "" when it holds none or several.
func embedded(p *schema) string {
	cands := []*schema{p}
	if len(p.AnyOf) > 0 {
		cands = p.AnyOf
	}
	name := ""
	for _, c := range cands {
		if c.Ref == "" {
			continue
		}
		n := refName(c)
		if isResource(n) {
			continue
		}
		if name != "" {
			return ""
		}
		name = n
	}
	return name
}

func isStringMap(p *schema) bool {
	if p.Type != "object" || len(p.AdditionalProperties) == 0 {
		return false
	}
	var ap schema
	if json.Unmarshal(p.AdditionalProperties, &ap) != nil {
		return false
	}
	return ap.Type == "string"
}

const (
	fillNone = iota
	fillNull
	fillEmptyMap
	fillEmptyArray
	fillEmptyObject
)

type prop struct {
	name   string
	fill   int
	nested string // embedded sub-object schema, for recursion
}

var memo = map[string]*bool{}

// fillable reports whether every property of an embedded schema can be
// filled with its unset value, so an absent sub-object can be returned as
// Stripe returns it unset.
func fillable(name string) bool {
	if v := memo[name]; v != nil {
		return *v
	}
	f := false
	memo[name] = &f // breaks cycles
	ok := len(schemas[name].Properties) > 0
	for _, p := range props(name) {
		if p.fill == fillNone {
			ok = false
		}
	}
	memo[name] = &ok
	return ok
}

func props(name string) []prop {
	s := schemas[name]
	skip := map[string]bool{"id": true, "object": true}
	for _, n := range includable[name] {
		skip[n] = true
	}
	var out []prop
	for _, k := range sortedKeys(s.Properties) {
		if skip[k] {
			continue
		}
		p := s.Properties[k]
		pr := prop{name: k, nested: embedded(p)}
		if p.Type == "array" && p.Items != nil {
			pr.nested = embedded(p.Items)
		}
		switch {
		case p.Nullable:
			pr.fill = fillNull
		case isStringMap(p):
			pr.fill = fillEmptyMap
		case p.Type == "array":
			pr.fill = fillEmptyArray
		case pr.nested != "" && p.Type != "array" && fillable(pr.nested):
			pr.fill = fillEmptyObject
		}
		out = append(out, pr)
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func main() {
	specPath := flag.String("spec", "", "path to the pinned Stripe OpenAPI spec")
	out := flag.String("o", "response_schema_gen.go", "output file")
	flag.Parse()
	raw, err := os.ReadFile(*specPath)
	if err != nil {
		log.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	var sp spec
	if err := json.Unmarshal(raw, &sp); err != nil {
		log.Fatal(err)
	}
	schemas = sp.Components.Schemas

	objectSchema := map[string]string{}
	for _, name := range sortedKeys(schemas) {
		if !isResource(name) || strings.HasPrefix(name, "deleted_") {
			continue
		}
		obj := schemas[name].Properties["object"].Enum[0].(string)
		if _, ok := objectSchema[obj]; !ok || schemas[name].ResourceID == obj {
			objectSchema[obj] = name
		}
	}

	// Walk every schema reachable from a served object through embedded
	// sub-objects.
	want := map[string]bool{}
	var visit func(string)
	visit = func(n string) {
		if want[n] {
			return
		}
		want[n] = true
		for _, p := range props(n) {
			if p.nested != "" {
				visit(p.nested)
			}
		}
	}
	objects := map[string]string{}
	for _, obj := range served {
		n, ok := objectSchema[obj]
		if !ok {
			log.Fatalf("no schema for object %q", obj)
		}
		objects[obj] = n
		visit(n)
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by gen_response_schema.go from %s (sha256 %s). DO NOT EDIT.\n\n", specID, hex.EncodeToString(sum[:]))
	b.WriteString("package api\n\n")
	b.WriteString("// objectSchemas maps an object value to its schema in the spec.\nvar objectSchemas = map[string]string{\n")
	for _, k := range sortedKeys(objects) {
		fmt.Fprintf(&b, "%q: %q,\n", k, objects[k])
	}
	b.WriteString("}\n\n")
	b.WriteString("// responseSchemas lists, per schema, the properties Stripe returns and how\n// an unset one is filled.\nvar responseSchemas = map[string][]responseProp{\n")
	names := map[int]string{fillNone: "fillNone", fillNull: "fillNull", fillEmptyMap: "fillEmptyMap", fillEmptyArray: "fillEmptyArray", fillEmptyObject: "fillEmptyObject"}
	for _, n := range sortedKeys(want) {
		fmt.Fprintf(&b, "%q: {\n", n)
		for _, p := range props(n) {
			if p.fill == fillNone && p.nested == "" {
				continue
			}
			fmt.Fprintf(&b, "{%q, %s, %q},\n", p.name, names[p.fill], p.nested)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	src, err := format.Source(b.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}
