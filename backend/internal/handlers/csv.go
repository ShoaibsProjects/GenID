package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/middleware"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) PreviewCSVImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CSVData string `json:"csv_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	if len(req.CSVData) == 0 {
		respondError(w, http.StatusBadRequest, "No CSV data provided")
		return
	}

	reader := csv.NewReader(strings.NewReader(req.CSVData))
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		respondError(w, http.StatusBadRequest, "Failed to parse CSV headers: "+err.Error())
		return
	}

	// Read up to 10 sample rows
	var sampleRows []map[string]string
	for i := 0; i < 10; i++ {
		record, err := reader.Read()
		if err != nil {
			break
		}
		row := map[string]string{}
		for j, h := range headers {
			if j < len(record) {
				row[strings.TrimSpace(h)] = strings.TrimSpace(record[j])
			}
		}
		sampleRows = append(sampleRows, row)
	}

	// Suggest mappings based on header name matching
	suggestedMapping := map[string]string{}
	knownFields := map[string]string{
		"email": "email", "emailaddress": "email", "mail": "email", "e-mail": "email",
		"display_name": "display_name", "displayname": "display_name", "name": "display_name",
		"first_name": "first_name", "firstname": "first_name", "givenname": "first_name",
		"last_name": "last_name", "lastname": "last_name", "surname": "last_name",
		"department": "department", "dept": "department",
		"title": "title", "jobtitle": "title", "position": "title",
		"employee_id": "employee_id", "employeeid": "employee_id", "id": "employee_id",
		"manager": "manager_id", "manager_id": "manager_id",
		"phone": "phone", "mobile": "phone", "telephone": "phone",
		"status": "status", "type": "type", "source": "source",
	}
	for _, h := range headers {
		h = strings.TrimSpace(h)
		lower := strings.ToLower(strings.ReplaceAll(h, " ", "_"))
		if mapped, ok := knownFields[lower]; ok {
			suggestedMapping[h] = mapped
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"columns":           headers,
		"sample_rows":       sampleRows,
		"row_count_preview": len(sampleRows),
		"suggested_mapping": suggestedMapping,
	})
}

func (h *Handler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CSVData       string            `json:"csv_data"`
		ColumnMapping map[string]string `json:"column_mapping"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	reader := csv.NewReader(strings.NewReader(req.CSVData))
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		respondError(w, http.StatusBadRequest, "Failed to parse CSV: "+err.Error())
		return
	}

	// Clean header names
	cleanHeaders := make([]string, len(headers))
	for i, h := range headers {
		cleanHeaders[i] = strings.TrimSpace(h)
	}

	created, updated, failed := 0, 0, 0
	var errs []string
	ctx := r.Context()

	for lineNum := 2; ; lineNum++ {
		record, err := reader.Read()
		if err != nil {
			break
		}

		// Build identity record from mapping
		email, displayName := "", ""
		customAttrs := map[string]string{}

		for i, h := range cleanHeaders {
			val := ""
			if i < len(record) {
				val = strings.TrimSpace(record[i])
			}
			if val == "" {
				continue
			}

			targetField, mapped := req.ColumnMapping[h]
			if !mapped {
				// Unmapped column → custom attribute
				key := strings.ToLower(strings.ReplaceAll(h, " ", "_"))
				customAttrs[key] = val
				continue
			}

			switch targetField {
			case "email":
				email = val
			case "display_name":
				displayName = val
			case "department":
				customAttrs["department"] = val
			case "title":
				customAttrs["title"] = val
			case "employee_id":
				customAttrs["employee_id"] = val
			case "first_name":
				customAttrs["first_name"] = val
			case "last_name":
				customAttrs["last_name"] = val
			case "phone":
				customAttrs["phone"] = val
			case "source":
				customAttrs["source"] = val
			case "status":
				customAttrs["status"] = val
			case "type":
				customAttrs["type"] = val
			default:
				customAttrs[targetField] = val
			}
		}

		if email == "" || displayName == "" {
			failed++
			errs = append(errs, fmt.Sprintf("row %d: missing email or display_name", lineNum))
			continue
		}

		status := "active"
		if s, ok := customAttrs["status"]; ok {
			status = s
			delete(customAttrs, "status")
		}

		attrsJSON, _ := json.Marshal(customAttrs)

		tag, err := h.DB(r.Context()).Exec(ctx, `
			INSERT INTO identities (id, tenant_id, email, display_name, status, source, department, attributes)
			VALUES ($1, $2, $3, $4, $5, 'hris', $6, $7)
			ON CONFLICT (tenant_id, email) DO UPDATE SET
				display_name = EXCLUDED.display_name, status = EXCLUDED.status,
				department = EXCLUDED.department, attributes = EXCLUDED.attributes,
				updated_at = NOW()
		`, uuid.New().String(), middleware.TenantIDFromContext(r.Context()),
			email, displayName, status,
			customAttrs["department"], json.RawMessage(attrsJSON))

		if err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("row %d (%s): %v", lineNum, email, err))
			continue
		}

		if tag.Insert() {
			created++
		} else {
			updated++
		}
	}

	h.AuditStore().Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity",
		Message: fmt.Sprintf("CSV import: %d created, %d updated, %d failed", created, updated, failed),
		Tags:    []string{"identity", "csv", "import"},
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "completed",
		"created": created, "updated": updated, "failed": failed,
		"errors": errs,
	})
}

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB(r.Context()).Query(r.Context(), `
		SELECT email, display_name, COALESCE(department,''), COALESCE(employee_id,''),
		       source, status, type, risk_score,
		       COALESCE(attributes::text,'{}'), created_at, updated_at
		FROM identities ORDER BY created_at DESC
	`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=genid-identities.csv")

	writer := csv.NewWriter(w)
	writer.Write([]string{
		"email", "display_name", "department", "employee_id", "source",
		"status", "type", "risk_score", "custom_attributes", "created_at", "updated_at",
	})

	for rows.Next() {
		var email, name, dept, empID, source, status, idType, attrs string
		var risk float64
		var created, updated time.Time
		if err := rows.Scan(&email, &name, &dept, &empID, &source, &status, &idType, &risk, &attrs, &created, &updated); err != nil {
			continue
		}
		writer.Write([]string{
			email, name, dept, empID, source, status, idType,
			fmt.Sprintf("%.2f", risk), attrs,
			created.Format("2006-01-02 15:04:05"), updated.Format("2006-01-02 15:04:05"),
		})
	}
	rows.Close()
	writer.Flush()
}

// ─── Group CRUD Handlers ────────────────────────────────────
