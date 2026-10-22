package ecore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// DashboardsFormTitle is the system form whose actors are charts/dashboards.
const DashboardsFormTitle = "Dashboards"

// dashboardFormCache memoizes, per API base URL + form id, whether that form is
// the Dashboards system form. A form's type and title do not change, so the entry
// is cached for the process lifetime (no TTL). The key needs no workspace (GET
// /forms/{formId} resolves the form by its own id) but must carry the base URL:
// form ids are only unique within one backend database, so after set-environment
// the same id can name a different form. Failed lookups are not cached.
var dashboardFormCache sync.Map // string (baseURL|formId) → bool

// FormGetter issues an authenticated GET for a /papi path relative to the
// caller's base URL and returns the response body.
type FormGetter func(ctx context.Context, path string, query url.Values) ([]byte, error)

// IsDashboardForm reports whether formID is the Dashboards *system* form, matching
// the backend's own rule (pong-server getSystemForms: type == "system" and a
// case-insensitive title match on "dashboards"). It resolves the form by its id —
// GET /forms/{formId} needs no active workspace and cannot be fooled by a custom
// form that merely shares the title. baseURL only scopes the cache; get performs
// the request. It is shared by the createActor/updateActor guards
// (internal/tools) and the pushGraphFile guard (internal/engines/graph); callers
// fail open on error so a transient lookup failure never blocks a legitimate write.
func IsDashboardForm(ctx context.Context, baseURL string, formID int, get FormGetter) (bool, error) {
	if formID <= 0 {
		return false, nil
	}
	key := strings.TrimSuffix(baseURL, "/") + "|" + strconv.Itoa(formID)
	if v, ok := dashboardFormCache.Load(key); ok {
		return v.(bool), nil
	}
	resp, err := get(ctx, fmt.Sprintf("/forms/%d", formID), url.Values{"filter": {"type,title"}})
	if err != nil {
		return false, err
	}
	var out struct {
		Data struct {
			Type  string `json:"type"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return false, err
	}
	isDash := out.Data.Type == "system" && strings.EqualFold(out.Data.Title, DashboardsFormTitle)
	dashboardFormCache.Store(key, isDash)
	return isDash, nil
}
