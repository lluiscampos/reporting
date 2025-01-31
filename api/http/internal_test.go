// Copyright 2021 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/mendersoftware/go-lib-micro/rest.utils"

	"github.com/mendersoftware/reporting/app/reporting"
	mapp "github.com/mendersoftware/reporting/app/reporting/mocks"
	"github.com/mendersoftware/reporting/model"
)

var contextMatcher = mock.MatchedBy(func(_ context.Context) bool { return true })

func TestStatus(t *testing.T) {
	t.Parallel()
	router := NewRouter(nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, URIInternal+URILiveliness, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestInternalSearch(t *testing.T) {
	t.Parallel()
	var newSearchParamMatcher = func(expected *model.SearchParams) interface{} {
		return mock.MatchedBy(func(actual *model.SearchParams) bool {
			if expected.Page <= 0 {
				expected.Page = ParamPageDefault
			}
			if expected.PerPage <= 0 {
				expected.PerPage = ParamPerPageDefault
			}
			if assert.NotNil(t, actual) {
				return assert.Equal(t, *expected, *actual)
			}
			return false
		})
	}
	type testCase struct {
		Name string

		App      func(*testing.T, testCase) *mapp.App
		TenantID string
		Params   *model.SearchParams

		Code     int
		Response interface{}
	}
	testCases := []testCase{{
		Name: "ok",

		App: func(t *testing.T, self testCase) *mapp.App {
			app := new(mapp.App)

			app.On("InventorySearchDevices",
				contextMatcher,
				newSearchParamMatcher(self.Params)).
				Return(self.Response, 0, nil)
			return app
		},
		TenantID: "123456789012345678901234",
		Params: &model.SearchParams{
			PerPage: 10,
			Page:    2,
			Filters: []model.FilterPredicate{{
				Scope:     "inventory",
				Attribute: "ip4",
				Type:      "$exists",
				Value:     true,
			}},
			Sort: []model.SortCriteria{{
				Scope:     "inventory",
				Attribute: "ip4",
				Order:     "asc",
			}},
			TenantID: "123456789012345678901234",
		},

		Code: http.StatusOK,
		Response: []model.InvDevice{{
			ID: model.DeviceID("5975e1e6-49a6-4218-a46d-f181154a98cc"),
			Attributes: model.DeviceAttributes{{
				Scope: "inventory",
				Name:  "ip4",
				Value: "10.0.0.2",
			}, {
				Scope: "system",
				Name:  "group",
				Value: "develop",
			}},
			Group:     model.GroupName("dev-set"),
			CreatedTs: time.Now().Add(-time.Hour),
			UpdatedTs: time.Now().Add(-time.Minute),
			Revision:  3,
		}, {
			ID: model.DeviceID("83bce0e4-c4c0-4995-b8b7-f056da7fc8f6"),

			Attributes: model.DeviceAttributes{{
				Scope: "inventory",
				Name:  "ip4",
				Value: "10.0.0.2",
			}, {
				Scope: "system",
				Name:  "group",
				Value: "prod_horse",
			}},
			Group:     model.GroupName("prod_horse"),
			CreatedTs: time.Now().Add(-2 * time.Hour),
			UpdatedTs: time.Now().Add(-5 * time.Minute),
			Revision:  120,
		}},
	}, {
		Name: "ok, empty result",

		App: func(t *testing.T, self testCase) *mapp.App {
			app := new(mapp.App)

			app.On("InventorySearchDevices",
				contextMatcher,
				newSearchParamMatcher(self.Params)).
				Return([]model.InvDevice{}, 0, nil)
			return app
		},
		TenantID: "123456789012345678901234",
		Params: &model.SearchParams{
			TenantID: "123456789012345678901234",
		},

		Code:     http.StatusOK,
		Response: []model.InvDevice{},
	}, {
		Name: "error, malformed request body",

		TenantID: "123456789012345678901234",
		Params: &model.SearchParams{
			Filters: []model.FilterPredicate{{
				Scope:     "secret-attrs",
				Type:      "$maybethiswillfindsomethinginterresting",
				Attribute: "rootpwd",
				Value:     true,
			}},
			TenantID: "123456789012345678901234",
		},
		Code:     http.StatusBadRequest,
		Response: rest.Error{Err: "malformed request body: type: must be a valid value."},
	}, {
		Name: "error, internal app error",

		App: func(t *testing.T, self testCase) *mapp.App {
			app := new(mapp.App)

			app.On("InventorySearchDevices",
				contextMatcher,
				newSearchParamMatcher(self.Params)).
				Return(nil, 0, errors.New("internal error"))
			return app
		},
		TenantID: "123456789012345678901234",
		Params: &model.SearchParams{
			PerPage: 10,
			Page:    2,
			Filters: []model.FilterPredicate{{
				Scope:     "inventory",
				Attribute: "ip4",
				Type:      "$exists",
				Value:     true,
			}},
			Sort: []model.SortCriteria{{
				Scope:     "inventory",
				Attribute: "ip4",
				Order:     "asc",
			}},
			TenantID: "123456789012345678901234",
		},

		Code:     http.StatusInternalServerError,
		Response: rest.Error{Err: "internal error"},
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			var app *mapp.App
			if tc.App == nil {
				app = new(mapp.App)
			} else {
				app = tc.App(t, tc)
			}
			defer app.AssertExpectations(t)
			router := NewRouter(app)

			b, _ := json.Marshal(tc.Params)
			repl := strings.NewReplacer(":tenant_id", tc.TenantID)
			req, _ := http.NewRequest(
				http.MethodPost,
				URIInternal+repl.Replace(URIInventorySearchInternal),
				bytes.NewReader(b),
			)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.Code, w.Code)

			switch res := tc.Response.(type) {
			case []model.InvDevice:
				b, _ := json.Marshal(res)
				assert.JSONEq(t, string(b), w.Body.String())

			case rest.Error:
				var actual rest.Error
				dec := json.NewDecoder(w.Body)
				dec.DisallowUnknownFields()
				err := dec.Decode(&actual)
				if assert.NoError(t, err, "response schema did not match expected rest.Error") {
					assert.EqualError(t, res, actual.Error())
				}

			case nil:
				assert.Empty(t, w.Body.String())

			default:
				panic("[TEST ERR] Dunno what to compare!")
			}

		})
	}
}

func TestReindex(t *testing.T) {
	t.Parallel()
	type testCase struct {
		Name string

		App      func(*testing.T, testCase) *mapp.App
		TenantID string
		DeviceID string
		Q        url.Values

		Code     int
		Response interface{}
	}
	testCases := []testCase{{
		Name: "ok",

		App: func(t *testing.T, self testCase) *mapp.App {
			app := new(mapp.App)
			app.On("Reindex", contextMatcher, self.TenantID,
				self.DeviceID, "inventory").
				Return(nil)
			return app
		},
		TenantID: "123456789012345678901234",
		DeviceID: "3ff2da3a-342f-45a1-b7f7-d79c080db5f1",
		Q: url.Values{
			"service": []string{"inventory"},
		},

		Code:     http.StatusAccepted,
		Response: nil,
	}, {
		Name: "error, service unknown",

		App: func(t *testing.T, self testCase) *mapp.App {
			app := new(mapp.App)
			app.On("Reindex", contextMatcher, self.TenantID,
				self.DeviceID, "elasticbogaloo").
				Return(reporting.ErrUnknownService)
			return app
		},
		TenantID: "123456789012345678901234",
		DeviceID: "3ff2da3a-342f-45a1-b7f7-d79c080db5f1",
		Q: url.Values{
			"service": []string{"elasticbogaloo"},
		},

		Code: http.StatusBadRequest,
		Response: rest.Error{
			Err: reporting.ErrUnknownService.Error(),
		},
	}, {
		Name: "error, internal error",

		App: func(t *testing.T, self testCase) *mapp.App {
			app := new(mapp.App)
			app.On("Reindex", contextMatcher, self.TenantID,
				self.DeviceID, "").
				Return(errors.New("internal error"))
			return app
		},
		TenantID: "123456789012345678901234",
		DeviceID: "3ff2da3a-342f-45a1-b7f7-d79c080db5f1",

		Code: http.StatusInternalServerError,
		Response: rest.Error{
			Err: http.StatusText(http.StatusInternalServerError),
		},
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			var app *mapp.App
			if tc.App == nil {
				app = new(mapp.App)
			} else {
				app = tc.App(t, tc)
			}
			defer app.AssertExpectations(t)
			router := NewRouter(app)

			repl := strings.NewReplacer(
				":tenant_id", tc.TenantID,
				":device_id", tc.DeviceID,
			)
			req, _ := http.NewRequest(
				http.MethodPost,
				URIInternal+repl.Replace(URIReindexInternal),
				nil,
			)
			req.URL.RawQuery = tc.Q.Encode()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.Code, w.Code)

			switch typ := tc.Response.(type) {
			case rest.Error:
				var actual rest.Error
				dec := json.NewDecoder(w.Body)
				dec.DisallowUnknownFields()
				err := dec.Decode(&actual)
				if assert.NoError(t, err, "unexpected response schema") {
					assert.EqualError(t, actual, typ.Error())
				}

			case nil:
				assert.Empty(t, w.Body.Bytes())
			default:
				panic("[TEST ERR] Dunno what to compare!")
			}
		})
	}
}
