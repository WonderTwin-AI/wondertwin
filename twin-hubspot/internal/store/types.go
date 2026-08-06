// Package store defines the HubSpot twin's state types and in-memory store.
package store

// CRMObject is the universal HubSpot CRM object. All object types (contacts,
// companies, deals, tickets, notes, calls, emails, meetings, tasks, etc.)
// share this structure. The ObjectType field distinguishes them.
type CRMObject struct {
	ID         string            `json:"id"`
	Properties map[string]string `json:"properties"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
	Archived   bool              `json:"archived"`
	ObjectType string            `json:"-"` // internal: "contacts", "companies", etc.
}

// Association links two CRM objects.
type Association struct {
	ID                  string `json:"id"`
	FromObjectType      string `json:"-"`
	FromObjectID        string `json:"-"`
	ToObjectType        string `json:"-"`
	ToObjectID          string `json:"-"`
	AssociationCategory string `json:"associationCategory"` // "HUBSPOT_DEFINED" or "USER_DEFINED"
	AssociationTypeID   int    `json:"associationTypeId"`
}

// Property defines a CRM property schema.
type Property struct {
	Name        string           `json:"name"`
	Label       string           `json:"label"`
	Type        string           `json:"type"`      // "string", "number", "date", "datetime", "enumeration", "bool"
	FieldType   string           `json:"fieldType"` // "text", "textarea", "number", "select", etc.
	GroupName   string           `json:"groupName"`
	Description string           `json:"description,omitempty"`
	Options     []PropertyOption `json:"options,omitempty"`
	ObjectType  string           `json:"-"` // internal: which object type this property belongs to
}

// PropertyOption is an option for an enumeration property.
type PropertyOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// PropertyGroup groups related properties.
type PropertyGroup struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	ObjectType string `json:"-"`
}

// Pipeline represents a deal or ticket pipeline.
type Pipeline struct {
	ID         string          `json:"id"`
	Label      string          `json:"label"`
	Stages     []PipelineStage `json:"stages"`
	ObjectType string          `json:"-"` // "deals" or "tickets"
	CreatedAt  string          `json:"createdAt"`
	UpdatedAt  string          `json:"updatedAt"`
}

// PipelineStage represents a stage within a pipeline.
type PipelineStage struct {
	ID           string         `json:"id"`
	Label        string         `json:"label"`
	DisplayOrder int            `json:"displayOrder"`
	Metadata     map[string]any `json:"metadata,omitempty"` // probability for deals, ticketState for tickets
}

// Owner represents a HubSpot owner (user).
type Owner struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	UserID    int64  `json:"userId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// List represents a HubSpot list (segment).
type List struct {
	ID             string   `json:"listId"`
	Name           string   `json:"name"`
	ObjectTypeID   string   `json:"objectTypeId"`   // "0-1" contacts, "0-2" companies, etc.
	ProcessingType string   `json:"processingType"` // "MANUAL" or "DYNAMIC"
	MemberIDs      []string `json:"-"`              // internal: member object IDs
}

// Webhook represents a HubSpot webhook subscription.
type Webhook struct {
	ID               string `json:"id"`
	SubscriptionType string `json:"subscriptionType"` // "contact.creation", "deal.propertyChange", etc.
	PropertyName     string `json:"propertyName,omitempty"`
	Active           bool   `json:"active"`
	CreatedAt        string `json:"createdAt"`
}

// Form represents a HubSpot form.
type Form struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FormType  string `json:"formType"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
