"use client"
import { useState, useRef } from "react"

const FIELD_OPTIONS = [
  { value: "email", label: "Email *", required: true },
  { value: "display_name", label: "Display Name *", required: true },
  { value: "first_name", label: "First Name" },
  { value: "last_name", label: "Last Name" },
  { value: "department", label: "Department" },
  { value: "title", label: "Job Title" },
  { value: "employee_id", label: "Employee ID" },
  { value: "phone", label: "Phone" },
  { value: "source", label: "Source" },
  { value: "status", label: "Status" },
  { value: "type", label: "Type" },
  { value: "manager_id", label: "Manager ID" },
]

export default function CSVPage() {
  const [csvData, setCsvData] = useState("")
  const [preview, setPreview] = useState<any>(null)
  const [mapping, setMapping] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<any>(null)
  const [dragging, setDragging] = useState(false)
  const [csvName, setCsvName] = useState("")
  const fileRef = useRef<HTMLInputElement>(null)

  async function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setCsvName(file.name)
    const text = await file.text()
    setCsvData(text)
    await previewCSV(text)
  }

  async function previewCSV(data: string) {
    setLoading(true)
    try {
      const res = await fetch("/api/v1/identities/csv/preview", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ csv_data: data }) })
      const d = await res.json()
      setPreview(d)
      setMapping(d.suggested_mapping || {})
    } catch (e: any) { alert("Preview failed: " + e.message) }
    finally { setLoading(false) }
  }

  async function handleImport() {
    if (!csvData) return
    setLoading(true); setResult(null)
    try {
      const res = await fetch("/api/v1/identities/csv/import", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ csv_data: csvData, column_mapping: mapping }) })
      const d = await res.json()
      setResult(d)
    } catch (e: any) { setResult({ status: "error", errors: [e.message] }) }
    finally { setLoading(false) }
  }

  function handleExport() {
    window.open("/api/v1/identities/csv/export", "_blank")
  }

  function handleDragOver(e: React.DragEvent) { e.preventDefault(); setDragging(true) }
  function handleDragLeave() { setDragging(false) }
  function handleDrop(e: React.DragEvent) {
    e.preventDefault(); setDragging(false)
    const file = e.dataTransfer.files[0]
    if (!file) return
    setCsvName(file.name)
    const reader = new FileReader()
    reader.onload = (ev) => {
      const text = ev.target?.result as string
      setCsvData(text)
      previewCSV(text)
    }
    reader.readAsText(file)
  }

  const unmappedColumns = preview?.columns?.filter((c: string) => !mapping[c]) || []

  return (
    <div className="space-y-4" style={{ maxWidth: 1100, margin: '0 auto' }}>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">CSV Identity Import</h1>
          <p className="text-sm text-gray-400 mt-1">Upload HR data, map columns to identity fields, import in bulk</p>
        </div>
        <div className="flex gap-2">
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={handleExport}>Export CSV</button>
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={() => window.open("/api/v1/identities/csv/export", "_blank")}>
            Download Template
          </button>
        </div>
      </div>

      {/* Upload Zone */}
      {!preview && (
        <div
          onDragOver={handleDragOver} onDragLeave={handleDragLeave} onDrop={handleDrop}
          onClick={() => fileRef.current?.click()}
          className="glass-card p-12 text-center cursor-pointer transition-all"
          style={{
            borderStyle: 'dashed',
            borderColor: dragging ? 'rgba(245,158,11,0.5)' : 'rgba(255,255,255,0.08)',
            background: dragging ? 'rgba(245,158,11,0.03)' : undefined,
          }}
        >
          <div style={{ fontSize: 48, marginBottom: 12 }}>📄</div>
          <h3 className="text-lg font-semibold text-white mb-1">Drop CSV File Here</h3>
          <p className="text-sm text-gray-400 mb-3">or click to browse — .csv files with headers</p>
          <input ref={fileRef} type="file" accept=".csv" onChange={handleFile} className="hidden" />
          <p className="text-xs text-gray-500">Includes: email, display_name, department, title, employee_id, and custom attributes</p>
        </div>
      )}

      {/* Column Mapping */}
      {preview && (
        <div className="space-y-4 animate-slide-in">
          <div className="flex items-center justify-between">
            <div>
              <span className="text-sm text-white font-medium">{csvName}</span>
              <span className="text-xs text-gray-500 ml-2">{preview.columns.length} columns · {preview.row_count_preview} sample rows</span>
            </div>
            <div className="flex gap-2">
              <button className="btn-secondary text-xs px-3 py-1.5" onClick={() => { setPreview(null); setCsvData(""); setResult(null) }}>Clear</button>
              <button className="btn-primary text-xs px-4 py-1.5" onClick={handleImport} disabled={loading}>
                {loading ? "Importing..." : `Import ${preview.row_count_preview}+ Records`}
              </button>
            </div>
          </div>

          {result && (
            <div className={`p-4 rounded-lg ${result.status === "completed" && result.failed === 0 ? "bg-emerald-500/5 border border-emerald-500/20" : "bg-amber-500/5 border border-amber-500/20"}`}>
              <div className="flex gap-6 text-sm">
                <div><span className="text-gray-400">Created:</span> <span className="text-emerald-400 font-bold">{result.created}</span></div>
                <div><span className="text-gray-400">Updated:</span> <span className="text-amber-400 font-bold">{result.updated}</span></div>
                <div><span className="text-gray-400">Failed:</span> <span className={result.failed > 0 ? "text-red-400 font-bold" : "text-gray-500"}>{result.failed}</span></div>
              </div>
              {result.errors?.length > 0 && (
                <div className="mt-2 text-xs text-red-400 space-y-0.5">
                  {result.errors.slice(0, 5).map((e: string, i: number) => <div key={i}>{e}</div>)}
                </div>
              )}
            </div>
          )}

          {/* Mapping UI */}
          <div className="glass-card p-4">
            <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-3">Column Mapping</h3>
            <div className="grid grid-cols-2 gap-3">
              {preview.columns.map((col: string) => (
                <div key={col} className="flex items-center gap-3 p-2 rounded" style={{ background: mapping[col] ? 'rgba(52,211,153,0.03)' : 'rgba(245,158,11,0.03)', border: `1px solid ${mapping[col] ? 'rgba(52,211,153,0.10)' : 'rgba(245,158,11,0.10)'}` }}>
                  <span className="text-xs text-gray-200 font-mono w-32 truncate" title={col}>{col}</span>
                  <span className="text-gray-500">→</span>
                  <select
                    className="input text-xs py-1 flex-1"
                    value={mapping[col] || ""}
                    onChange={e => setMapping({ ...mapping, [col]: e.target.value })}
                  >
                    <option value="">— Custom Attribute —</option>
                    {FIELD_OPTIONS.map(f => (
                      <option key={f.value} value={f.value}>{f.label}{f.required ? " *" : ""}</option>
                    ))}
                  </select>
                  {!mapping[col] && <span className="text-xs text-amber-400 font-mono">attr</span>}
                </div>
              ))}
            </div>
          </div>

          {/* Data Preview */}
          <div className="glass-card overflow-hidden">
            <h3 className="px-4 pt-4 text-sm font-semibold text-gray-300 uppercase tracking-wider">Data Preview</h3>
            <div className="overflow-x-auto p-4">
              <table className="w-full text-xs">
                <thead>
                  <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
                    {preview.columns.map((col: string) => (
                      <th key={col} className="text-left py-2 px-2 font-medium text-gray-500 uppercase whitespace-nowrap">
                        {col}
                        {mapping[col] && <span className="ml-1 text-emerald-400">→ {mapping[col]}</span>}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {preview.sample_rows.map((row: any, i: number) => (
                    <tr key={i} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)' }}>
                      {preview.columns.map((col: string) => (
                        <td key={col} className="py-1.5 px-2 text-gray-300 font-mono whitespace-nowrap max-w-[200px] truncate">
                          {row[col] || <span className="text-gray-600">—</span>}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Unmapped columns */}
          {unmappedColumns.length > 0 && (
            <div className="p-3 rounded" style={{ background: 'rgba(245,158,11,0.04)', border: '1px solid rgba(245,158,11,0.10)' }}>
              <span className="text-xs text-amber-400">
                {unmappedColumns.length} custom attribute{unmappedColumns.length > 1 ? "s" : ""}:{" "}
                {unmappedColumns.map((c: string, i: number) => (
                  <span key={c}>
                    <code className="text-amber-300">{c}</code>
                    {i < unmappedColumns.length - 1 ? ", " : ""}
                  </span>
                ))}
                {" "}will be stored as custom attributes
              </span>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
