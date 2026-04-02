package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all HubSpot twin state in memory.
type MemoryStore struct {
	Objects        *pkgstate.Store[CRMObject]
	Associations   *pkgstate.Store[Association]
	Properties     *pkgstate.Store[Property]
	PropertyGroups *pkgstate.Store[PropertyGroup]
	Pipelines      *pkgstate.Store[Pipeline]
	Owners         *pkgstate.Store[Owner]
	Lists          *pkgstate.Store[List]
	Webhooks       *pkgstate.Store[Webhook]
	Forms          *pkgstate.Store[Form]
	Clock          *pkgstate.Clock

	idCounter atomic.Int64
}

func New() *MemoryStore {
	return &MemoryStore{
		Objects:        pkgstate.New[CRMObject]("obj"),
		Associations:   pkgstate.New[Association]("assoc"),
		Properties:     pkgstate.New[Property]("prop"),
		PropertyGroups: pkgstate.New[PropertyGroup]("pg"),
		Pipelines:      pkgstate.New[Pipeline]("pipe"),
		Owners:         pkgstate.New[Owner]("owner"),
		Lists:          pkgstate.New[List]("list"),
		Webhooks:       pkgstate.New[Webhook]("wh"),
		Forms:          pkgstate.New[Form]("form"),
		Clock:          pkgstate.NewClock(),
	}
}

func (s *MemoryStore) NextID() string {
	return fmt.Sprintf("%d", s.idCounter.Add(1))
}

func (s *MemoryStore) Now() string {
	return s.Clock.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// ListObjectsByType returns all non-archived CRM objects of a given type.
func (s *MemoryStore) ListObjectsByType(objectType string) []CRMObject {
	return s.Objects.Filter(func(_ string, obj CRMObject) bool {
		return obj.ObjectType == objectType && !obj.Archived
	})
}

// GetObjectByID returns a CRM object by ID, checking type matches.
func (s *MemoryStore) GetObjectByID(objectType, id string) (*CRMObject, bool) {
	obj, ok := s.Objects.Get(id)
	if !ok || obj.ObjectType != objectType {
		return nil, false
	}
	return &obj, true
}

// SearchObjects does a simple property-based search.
func (s *MemoryStore) SearchObjects(objectType string, query string, filters []SearchFilter) []CRMObject {
	return s.Objects.Filter(func(_ string, obj CRMObject) bool {
		if obj.ObjectType != objectType || obj.Archived {
			return false
		}
		// Full-text query match
		if query != "" {
			found := false
			queryL := strings.ToLower(query)
			for _, v := range obj.Properties {
				if strings.Contains(strings.ToLower(v), queryL) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		// Filter match
		for _, f := range filters {
			val := obj.Properties[f.PropertyName]
			switch f.Operator {
			case "EQ":
				if val != f.Value {
					return false
				}
			case "NEQ":
				if val == f.Value {
					return false
				}
			case "CONTAINS_TOKEN":
				if !strings.Contains(strings.ToLower(val), strings.ToLower(f.Value)) {
					return false
				}
			case "HAS_PROPERTY":
				if val == "" {
					return false
				}
			case "NOT_HAS_PROPERTY":
				if val != "" {
					return false
				}
			}
		}
		return true
	})
}

// SearchFilter represents a single filter in a CRM search.
type SearchFilter struct {
	PropertyName string `json:"propertyName"`
	Operator     string `json:"operator"`
	Value        string `json:"value"`
}

// ListAssociations returns associations between two object types for a given object.
func (s *MemoryStore) ListAssociations(fromType, fromID, toType string) []Association {
	return s.Associations.Filter(func(_ string, a Association) bool {
		return a.FromObjectType == fromType && a.FromObjectID == fromID && a.ToObjectType == toType
	})
}

// ListPropertiesByType returns properties for an object type.
func (s *MemoryStore) ListPropertiesByType(objectType string) []Property {
	return s.Properties.Filter(func(_ string, p Property) bool {
		return p.ObjectType == objectType
	})
}

// ListPipelinesByType returns pipelines for an object type.
func (s *MemoryStore) ListPipelinesByType(objectType string) []Pipeline {
	return s.Pipelines.Filter(func(_ string, p Pipeline) bool {
		return p.ObjectType == objectType
	})
}

type stateSnapshot struct {
	Objects    map[string]CRMObject `json:"objects,omitempty"`
	Owners     map[string]Owner     `json:"owners,omitempty"`
	Pipelines  map[string]Pipeline  `json:"pipelines,omitempty"`
	Properties map[string]Property  `json:"properties,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Objects:    s.Objects.Snapshot(),
		Owners:     s.Owners.Snapshot(),
		Pipelines:  s.Pipelines.Snapshot(),
		Properties: s.Properties.Snapshot(),
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Objects != nil {
		s.Objects.LoadSnapshot(snap.Objects)
	}
	if snap.Owners != nil {
		s.Owners.LoadSnapshot(snap.Owners)
	}
	if snap.Pipelines != nil {
		s.Pipelines.LoadSnapshot(snap.Pipelines)
	}
	if snap.Properties != nil {
		s.Properties.LoadSnapshot(snap.Properties)
	}
	return nil
}

func (s *MemoryStore) Reset() {
	s.Objects.Reset()
	s.Associations.Reset()
	s.Properties.Reset()
	s.PropertyGroups.Reset()
	s.Pipelines.Reset()
	s.Owners.Reset()
	s.Lists.Reset()
	s.Webhooks.Reset()
	s.Forms.Reset()
	s.Clock.Reset()
	s.idCounter.Store(0)
}
