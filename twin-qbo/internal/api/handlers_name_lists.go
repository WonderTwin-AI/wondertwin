package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

// --- Employee ---

func (h *Handler) CreateOrUpdateEmployee(w http.ResponseWriter, r *http.Request) {
	var e store.Employee
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if e.Id != "" {
		existing, ok := h.store.Employees.Get(e.Id)
		if !ok { notFoundFault(w, "Employee", e.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, e.SyncToken); err != nil { staleSyncTokenFault(w); return }
		e.SyncToken = IncrementSyncToken(existing.SyncToken)
		e.MetaData.CreateTime = existing.MetaData.CreateTime
		e.MetaData.LastUpdatedTime = h.store.Now()
		e.Domain = "QBO"
		h.store.Employees.Set(e.Id, e)
	} else {
		e.Id = h.store.NextID(); e.SyncToken = "0"; e.Active = true; e.Domain = "QBO"
		e.MetaData = h.store.NewMetaData()
		h.store.Employees.Set(e.Id, e)
	}
	qboJSON(w, http.StatusOK, entityResponse("Employee", e))
}

func (h *Handler) GetEmployee(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	e, ok := h.store.Employees.Get(id)
	if !ok { notFoundFault(w, "Employee", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Employee", e))
}

// --- Class ---

func (h *Handler) CreateOrUpdateClass(w http.ResponseWriter, r *http.Request) {
	var c store.Class
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if c.Id != "" {
		existing, ok := h.store.Classes.Get(c.Id)
		if !ok { notFoundFault(w, "Class", c.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, c.SyncToken); err != nil { staleSyncTokenFault(w); return }
		c.SyncToken = IncrementSyncToken(existing.SyncToken)
		c.MetaData.CreateTime = existing.MetaData.CreateTime
		c.MetaData.LastUpdatedTime = h.store.Now()
		c.Domain = "QBO"
		h.store.Classes.Set(c.Id, c)
	} else {
		c.Id = h.store.NextID(); c.SyncToken = "0"; c.Active = true; c.Domain = "QBO"
		c.MetaData = h.store.NewMetaData()
		h.store.Classes.Set(c.Id, c)
	}
	qboJSON(w, http.StatusOK, entityResponse("Class", c))
}

func (h *Handler) GetClass(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.Classes.Get(id)
	if !ok { notFoundFault(w, "Class", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Class", c))
}

// --- Department ---

func (h *Handler) CreateOrUpdateDepartment(w http.ResponseWriter, r *http.Request) {
	var d store.Department
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if d.Id != "" {
		existing, ok := h.store.Departments.Get(d.Id)
		if !ok { notFoundFault(w, "Department", d.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, d.SyncToken); err != nil { staleSyncTokenFault(w); return }
		d.SyncToken = IncrementSyncToken(existing.SyncToken)
		d.MetaData.CreateTime = existing.MetaData.CreateTime
		d.MetaData.LastUpdatedTime = h.store.Now()
		d.Domain = "QBO"
		h.store.Departments.Set(d.Id, d)
	} else {
		d.Id = h.store.NextID(); d.SyncToken = "0"; d.Active = true; d.Domain = "QBO"
		d.MetaData = h.store.NewMetaData()
		h.store.Departments.Set(d.Id, d)
	}
	qboJSON(w, http.StatusOK, entityResponse("Department", d))
}

func (h *Handler) GetDepartment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.Departments.Get(id)
	if !ok { notFoundFault(w, "Department", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Department", d))
}

// --- Term ---

func (h *Handler) CreateOrUpdateTerm(w http.ResponseWriter, r *http.Request) {
	var t store.Term
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if t.Id != "" {
		existing, ok := h.store.Terms.Get(t.Id)
		if !ok { notFoundFault(w, "Term", t.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, t.SyncToken); err != nil { staleSyncTokenFault(w); return }
		t.SyncToken = IncrementSyncToken(existing.SyncToken)
		t.MetaData.CreateTime = existing.MetaData.CreateTime
		t.MetaData.LastUpdatedTime = h.store.Now()
		t.Domain = "QBO"
		h.store.Terms.Set(t.Id, t)
	} else {
		t.Id = h.store.NextID(); t.SyncToken = "0"; t.Active = true; t.Domain = "QBO"
		t.MetaData = h.store.NewMetaData()
		h.store.Terms.Set(t.Id, t)
	}
	qboJSON(w, http.StatusOK, entityResponse("Term", t))
}

func (h *Handler) GetTerm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, ok := h.store.Terms.Get(id)
	if !ok { notFoundFault(w, "Term", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Term", t))
}

// --- PaymentMethod ---

func (h *Handler) CreateOrUpdatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	var pm store.PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&pm); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if pm.Id != "" {
		existing, ok := h.store.PaymentMethods.Get(pm.Id)
		if !ok { notFoundFault(w, "PaymentMethod", pm.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, pm.SyncToken); err != nil { staleSyncTokenFault(w); return }
		pm.SyncToken = IncrementSyncToken(existing.SyncToken)
		pm.MetaData.CreateTime = existing.MetaData.CreateTime
		pm.MetaData.LastUpdatedTime = h.store.Now()
		pm.Domain = "QBO"
		h.store.PaymentMethods.Set(pm.Id, pm)
	} else {
		pm.Id = h.store.NextID(); pm.SyncToken = "0"; pm.Active = true; pm.Domain = "QBO"
		pm.MetaData = h.store.NewMetaData()
		h.store.PaymentMethods.Set(pm.Id, pm)
	}
	qboJSON(w, http.StatusOK, entityResponse("PaymentMethod", pm))
}

func (h *Handler) GetPaymentMethod(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pm, ok := h.store.PaymentMethods.Get(id)
	if !ok { notFoundFault(w, "PaymentMethod", id); return }
	qboJSON(w, http.StatusOK, entityResponse("PaymentMethod", pm))
}

// --- TaxCode (read-only in US) ---

func (h *Handler) GetTaxCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tc, ok := h.store.TaxCodes.Get(id)
	if !ok { notFoundFault(w, "TaxCode", id); return }
	qboJSON(w, http.StatusOK, entityResponse("TaxCode", tc))
}

// --- TaxRate (read-only in US) ---

func (h *Handler) GetTaxRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tr, ok := h.store.TaxRates.Get(id)
	if !ok { notFoundFault(w, "TaxRate", id); return }
	qboJSON(w, http.StatusOK, entityResponse("TaxRate", tr))
}

// --- Preferences ---

func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	prefs, ok := h.store.PreferencesStore.Get("1")
	if !ok {
		prefs = store.Preferences{
			Id: "1", SyncToken: "0", Domain: "QBO", MetaData: h.store.NewMetaData(),
		}
		h.store.PreferencesStore.Set("1", prefs)
	}
	qboJSON(w, http.StatusOK, entityResponse("Preferences", prefs))
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var prefs store.Preferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if prefs.Id == "" { prefs.Id = "1" }
	existing, ok := h.store.PreferencesStore.Get(prefs.Id)
	if !ok {
		existing = store.Preferences{Id: "1", SyncToken: "0", Domain: "QBO", MetaData: h.store.NewMetaData()}
	}
	if err := ValidateSyncToken(existing.SyncToken, prefs.SyncToken); err != nil { staleSyncTokenFault(w); return }
	prefs.SyncToken = IncrementSyncToken(existing.SyncToken)
	prefs.MetaData.CreateTime = existing.MetaData.CreateTime
	prefs.MetaData.LastUpdatedTime = h.store.Now()
	prefs.Domain = "QBO"
	h.store.PreferencesStore.Set(prefs.Id, prefs)
	qboJSON(w, http.StatusOK, entityResponse("Preferences", prefs))
}
