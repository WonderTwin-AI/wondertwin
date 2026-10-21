package seed

import "testing"

func TestLookupPasswordErrorCoversTheDocumentedCodesThatHaveASample(t *testing.T) {
	undocumented := map[string]bool{"COUNTRY_NOT_SUPPORTED": true, "USER_SHOULD_VERIFY_DEPOSIT_ALLOCATIONS": true}
	for code := range passwordCodes {
		e, ok := LookupPasswordError(code)
		if undocumented[code] {
			if ok {
				t.Errorf("%s has no published sample and must not be simulated", code)
			}
			continue
		}
		if !ok || e.Code != code || e.HTTP == 0 || e.Type == "" || e.Message == "" {
			t.Errorf("%s: %+v, found %v", code, e, ok)
		}
	}
}

func TestLookupItemErrorKnowsLoginRequiredButItIsNotAPassword(t *testing.T) {
	if e, ok := LookupItemError("ITEM_LOGIN_REQUIRED"); !ok || e.Type != "ITEM_ERROR" || e.DisplayMessage != nil {
		t.Fatalf("ITEM_LOGIN_REQUIRED = %+v, found %v", e, ok)
	}
	if _, ok := LookupPasswordError("ITEM_LOGIN_REQUIRED"); ok {
		t.Fatal("ITEM_LOGIN_REQUIRED is not a documented error password")
	}
	if e, _ := LookupItemError("INTERNAL_SERVER_ERROR"); e.HTTP != 500 || e.Type != "API_ERROR" {
		t.Fatalf("INTERNAL_SERVER_ERROR = %+v", e)
	}
}
