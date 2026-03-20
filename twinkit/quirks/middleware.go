package quirks

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// responseCapture wraps ResponseWriter to capture and potentially modify the response.
type responseCapture struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func newResponseCapture(w http.ResponseWriter) *responseCapture {
	return &responseCapture{
		ResponseWriter: w,
		status:         200,
	}
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.status = code
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	return rc.body.Write(b)
}

func (rc *responseCapture) Header() http.Header {
	return rc.ResponseWriter.Header()
}

// Middleware returns HTTP middleware that evaluates quirk rules before and
// after the handler runs. Pre-processing can modify requests or reject them.
// Post-processing can modify responses.
func Middleware(engine *Engine) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if engine == nil || engine.RuleCount() == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Build request context.
			var reqBody []byte
			var bodyMap map[string]any
			if r.Body != nil {
				reqBody, _ = io.ReadAll(r.Body)
				json.Unmarshal(reqBody, &bodyMap)
				r.Body = io.NopCloser(bytes.NewReader(reqBody))
			}

			queryMap := make(map[string]string)
			for k, v := range r.URL.Query() {
				if len(v) > 0 {
					queryMap[k] = v[0]
				}
			}

			headerMap := make(map[string]string)
			for k := range r.Header {
				headerMap[k] = r.Header.Get(k)
			}

			reqCtx := &RequestContext{
				Method:  r.Method,
				Path:    r.URL.Path,
				Headers: headerMap,
				Query:   queryMap,
				Body:    bodyMap,
			}

			// --- PRE-PROCESSING ---
			preActions := engine.EvaluatePre(reqCtx)
			for _, action := range preActions {
				switch action.Type {
				case "reject":
					status := action.StatusCode
					if status == 0 {
						status = http.StatusBadRequest
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					if action.Body != nil {
						json.NewEncoder(w).Encode(action.Body)
					}
					return

				case "delay":
					if action.Delay > 0 {
						time.Sleep(action.Delay)
					}

				case "modify_request":
					if bodyMap == nil {
						bodyMap = make(map[string]any)
					}
					bodyMap = ApplyRequestModifications(bodyMap, action.Modifications)
					modified, _ := json.Marshal(bodyMap)
					r.Body = io.NopCloser(bytes.NewReader(modified))
					r.ContentLength = int64(len(modified))
					reqCtx.Body = bodyMap
				}
			}

			// --- HANDLER ---
			capture := newResponseCapture(w)
			next.ServeHTTP(capture, r)

			// --- POST-PROCESSING ---
			respBody := capture.body.Bytes()
			var respBodyMap map[string]any
			json.Unmarshal(respBody, &respBodyMap)

			respCtx := &ResponseContext{
				Request:    reqCtx,
				StatusCode: capture.status,
				Body:       respBodyMap,
			}

			postActions := engine.EvaluatePost(respCtx)
			for _, action := range postActions {
				if action.Type == "modify_response" && respBodyMap != nil {
					respBodyMap = ApplyResponseModifications(respBodyMap, action.Modifications)
				}
			}

			// Write the (potentially modified) response.
			finalStatus := capture.status
			var finalBody []byte
			if len(postActions) > 0 && respBodyMap != nil {
				finalBody, _ = json.Marshal(respBodyMap)
			} else {
				finalBody = respBody
			}

			w.WriteHeader(finalStatus)
			w.Write(finalBody)
		})
	}
}
