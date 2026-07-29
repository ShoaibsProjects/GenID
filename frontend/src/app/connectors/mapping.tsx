"use client"

import { useState } from "react"

const GENID_FIELDS = ["Email", "First Name", "Last Name", "Department"]
const SOURCE_OPTIONS = ["mail", "givenName", "telecom_email", "department"]

export default function MappingEditor() {
  const [mappings, setMappings] = useState<Record<string, string>>(
    Object.fromEntries(GENID_FIELDS.map((f) => [f, ""]))
  )

  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 backdrop-blur-sm p-5">
      <h3 className="text-sm font-semibold text-white mb-4">Attribute Mapping</h3>

      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-zinc-800">
            <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">
              GenID Field
            </th>
            <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">
              Source Field
            </th>
          </tr>
        </thead>
        <tbody>
          {GENID_FIELDS.map((field) => (
            <tr key={field} className="border-b border-zinc-800/50 last:border-0">
              <td className="py-2.5 px-3 text-white">{field}</td>
              <td className="py-2 px-3">
                <select
                  className="bg-zinc-900 text-white border border-zinc-800 p-2 rounded-lg w-full text-sm focus:border-amber-500/50 focus:outline-none"
                  value={mappings[field]}
                  onChange={(e) =>
                    setMappings((prev) => ({ ...prev, [field]: e.target.value }))
                  }
                >
                  <option value="">-- Select --</option>
                  {SOURCE_OPTIONS.map((opt) => (
                    <option key={opt} value={opt}>
                      {opt}
                    </option>
                  ))}
                </select>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}