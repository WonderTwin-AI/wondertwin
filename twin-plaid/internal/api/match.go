package api

import (
	"slices"
	"strings"
	"unicode"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// The scorers below are the emulator's own rule, not Plaid's, whose algorithm
// is not published. Plaid documents the score bands, and these land in them:
// name 100 perfect, 99-85 strong, 84-70 partial, under 70 a mismatch; phone and
// email 100 perfect, 99-70 partial, under 70 a mismatch; address 100 perfect,
// 99-90 strong, 89-70 partial, under 70 weak. Where an account has several
// owners the best score wins, and a score is null when either side has no data.

type scoreOnly struct {
	Score *int `json:"score"`
}

type nameMatch struct {
	Score                      *int  `json:"score"`
	IsFirstNameOrLastNameMatch *bool `json:"is_first_name_or_last_name_match"`
	IsNicknameMatch            *bool `json:"is_nickname_match"`
	IsBusinessNameDetected     *bool `json:"is_business_name_detected"`
}

type addressMatch struct {
	Score             *int  `json:"score"`
	IsPostalCodeMatch *bool `json:"is_postal_code_match"`
}

func intp(n int) *int    { return &n }
func boolp(b bool) *bool { return &b }

// words lowercases s and splits it into letters and digits.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// businessWords are the keywords Plaid documents for spotting a business name.
var businessWords = []string{"llc", "inc", "ltd", "dba", "company", "business"}

func isBusiness(w []string) bool {
	return slices.ContainsFunc(w, func(x string) bool { return slices.Contains(businessWords, x) })
}

// scoreName scores a legal name against the account's owners' names.
func scoreName(given string, owners []store.Owner) nameMatch {
	g := words(given)
	best, haveAny := nameMatch{}, false
	for _, o := range owners {
		for _, n := range o.Names {
			a := words(n)
			if len(g) == 0 || len(a) == 0 {
				continue
			}
			haveAny = true
			biz := isBusiness(g) || isBusiness(a)
			score, firstLast := 0, g[0] == a[0] || g[len(g)-1] == a[len(a)-1]
			switch {
			case slices.Equal(g, a):
				score = 100
			case biz:
				score = 0 // business names are scored apart in Plaid; only an exact match counts here
			case sameSet(g, a):
				score = 95
			case subset(g, a) || subset(a, g):
				score = 90
			case firstLast:
				score = 75
			}
			if best.Score == nil || score > *best.Score {
				best = nameMatch{intp(score), boolp(firstLast), boolp(false), boolp(biz)}
			}
		}
	}
	if !haveAny {
		return nameMatch{}
	}
	return best
}

func subset(a, b []string) bool {
	return !slices.ContainsFunc(a, func(x string) bool { return !slices.Contains(b, x) })
}

func sameSet(a, b []string) bool { return subset(a, b) && subset(b, a) }

// digits keeps the last ten digits of a phone number, so +1 and formatting do not matter.
func digits(s string) string {
	var d []rune
	for _, r := range s {
		if unicode.IsDigit(r) {
			d = append(d, r)
		}
	}
	if len(d) > 10 {
		d = d[len(d)-10:]
	}
	return string(d)
}

func scorePhone(given string, owners []store.Owner) scoreOnly {
	if digits(given) == "" {
		return scoreOnly{}
	}
	best := scoreOnly{}
	for _, o := range owners {
		for _, p := range o.PhoneNumbers {
			s := 0
			if digits(p.Data) == digits(given) {
				s = 100
			}
			if best.Score == nil || s > *best.Score {
				best = scoreOnly{intp(s)}
			}
		}
	}
	return best
}

// emailKey lowercases an address and drops a "+tag" from the local part.
func emailKey(s string) (plain, stripped string) {
	plain = strings.ToLower(strings.TrimSpace(s))
	local, domain, ok := strings.Cut(plain, "@")
	if !ok {
		return plain, plain
	}
	local, _, _ = strings.Cut(local, "+")
	return plain, local + "@" + domain
}

func scoreEmail(given string, owners []store.Owner) scoreOnly {
	if strings.TrimSpace(given) == "" {
		return scoreOnly{}
	}
	gp, gs := emailKey(given)
	best := scoreOnly{}
	for _, o := range owners {
		for _, e := range o.Emails {
			ep, es := emailKey(e.Data)
			s := 0
			switch {
			case gp == ep:
				s = 100
			case gs == es:
				s = 80 // the same mailbox with a different + extension: a partial match
			}
			if best.Score == nil || s > *best.Score {
				best = scoreOnly{intp(s)}
			}
		}
	}
	return best
}

// postal compares the first five digits, so a ZIP+4 matches its ZIP.
func postal(s string) string {
	if d := strings.Join(words(s), ""); len(d) > 5 {
		return d[:5]
	}
	return strings.Join(words(s), "")
}

func scoreAddress(given store.AddressData, owners []store.Owner) addressMatch {
	if given.Street == "" && given.City == "" && given.Region == "" && given.PostalCode == "" {
		return addressMatch{}
	}
	best, haveAny := addressMatch{}, false
	for _, o := range owners {
		for _, a := range o.Addresses {
			haveAny = true
			street := strings.Join(words(given.Street), " ") == strings.Join(words(a.Data.Street), " ")
			city := strings.Join(words(given.City), " ") == strings.Join(words(a.Data.City), " ")
			region := strings.EqualFold(given.Region, a.Data.Region)
			post := postal(given.PostalCode) != "" && postal(given.PostalCode) == postal(a.Data.PostalCode)
			s := 0
			switch {
			case street && city && region && post:
				s = 100
			case street && post:
				s = 92
			case post:
				s = 75
			}
			var flag *bool
			if given.PostalCode != "" && a.Data.PostalCode != "" {
				flag = boolp(post)
			}
			if best.Score == nil || s > *best.Score {
				best = addressMatch{intp(s), flag}
			}
		}
	}
	if !haveAny {
		return addressMatch{}
	}
	return best
}
