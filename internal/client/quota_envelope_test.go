package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Synthetic regression for the live quota envelope discrepancy. Account values
// and raw live responses are never copied into fixtures.
func TestQuotaReadsAcceptRawAndWrappedButRejectIncompleteEnvelopes(t *testing.T) {
	for _, endpoint := range []string{"quota", "usage"} {
		data := `{"username":"owner","accounts":[]}`
		if endpoint == "quota" {
			data = `{"username":"owner","total_used":0,"total_limit":0,"percent_used":0,"breakdown":{"email":0,"web":0,"databases":0,"backups":0,"other":0},"grace_period":null,"updated_at":"2026-10-19"}`
		}
		for _, shape := range []string{"raw", "wrapped", "missing-data", "null-data", "false-success", "invalid-success", "partial-data"} {
			t.Run(endpoint+"/"+shape, func(t *testing.T) {
				body := data
				switch shape {
				case "wrapped":
					body = `{"success":true,"data":` + data + `}`
				case "missing-data":
					body = `{"success":true}`
				case "null-data":
					body = `{"success":true,"data":null}`
				case "false-success":
					body = `{"success":false,"data":` + data + `}`
				case "invalid-success":
					body = `{"success":"true","data":` + data + `}`
				case "partial-data":
					body = `{"success":true,"data":{"username":"owner"}}`
				}
				c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(body))
				}, "owner")
				var value any
				var err error
				if endpoint == "quota" {
					value, err = c.GetQuota(context.Background())
				} else {
					value, err = c.GetEmailUsage(context.Background())
				}
				if (err == nil) != (shape == "raw" || shape == "wrapped") {
					t.Fatal("quota envelope compatibility classification failed")
				}
				if err == nil {
					b, _ := json.Marshal(value)
					if string(b) != data {
						t.Fatal("quota envelope changed decoded values")
					}
				}
			})
		}
	}
}
