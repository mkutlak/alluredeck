import { execSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

const API_URL = process.env.ALLUREDECK_API_URL ?? 'http://localhost:5050/api/v1'

export class AllureDeckClient {
  private token = ''

  async login(username: string, password: string): Promise<void> {
    const res = await fetch(`${API_URL}/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
      redirect: 'manual',
    })
    if (!res.ok) throw new Error(`Login failed: ${res.status} ${await res.text()}`)

    // JWT is returned in a Set-Cookie header
    const cookies = res.headers.getSetCookie?.() ?? []
    const jwtCookie = cookies.find((c) => c.startsWith('jwt='))
    if (!jwtCookie) throw new Error('No jwt cookie in login response')
    this.token = jwtCookie.split('=')[1].split(';')[0]
  }

  private authHeaders(): Record<string, string> {
    return { Authorization: `Bearer ${this.token}` }
  }

  async uploadAllureResults(projectId: string, tarGzPath: string): Promise<void> {
    const body = fs.readFileSync(tarGzPath)
    const url = `${API_URL}/projects/${encodeURIComponent(projectId)}/results?force_project_creation=true`
    const res = await fetch(url, {
      method: 'POST',
      headers: { ...this.authHeaders(), 'Content-Type': 'application/gzip' },
      body,
    })
    if (!res.ok) throw new Error(`Upload allure results failed: ${res.status} ${await res.text()}`)
    console.log(`  Allure results uploaded to project "${projectId}"`)
  }

  async uploadPlaywrightReport(projectId: string, tarGzPath: string, buildNumber?: string): Promise<void> {
    const body = fs.readFileSync(tarGzPath)
    let url = `${API_URL}/projects/${encodeURIComponent(projectId)}/playwright?force_project_creation=true`
    if (buildNumber) url += `&build_number=${buildNumber}`
    const res = await fetch(url, {
      method: 'POST',
      headers: { ...this.authHeaders(), 'Content-Type': 'application/gzip' },
      body,
    })
    if (!res.ok)
      throw new Error(`Upload playwright report failed: ${res.status} ${await res.text()}`)
    console.log(`  Playwright report uploaded to project "${projectId}"`)
  }

  async getLatestReportId(projectId: string): Promise<string | null> {
    const res = await fetch(
      `${API_URL}/projects/${encodeURIComponent(projectId)}/reports?per_page=5`,
      { headers: this.authHeaders() },
    )
    if (!res.ok) return null
    const body = (await res.json()) as { data?: { reports?: Array<{ report_id: string }> } }
    const reports = body.data?.reports ?? []
    // Skip the virtual "latest" entry and return the first real report ID
    const real = reports.find((r) => r.report_id !== 'latest')
    return real?.report_id ?? null
  }

  async getReportCount(projectId: string): Promise<number> {
    const res = await fetch(
      `${API_URL}/projects/${encodeURIComponent(projectId)}/reports?per_page=100`,
      { headers: this.authHeaders() },
    )
    if (!res.ok) return 0
    const body = (await res.json()) as { data?: { reports?: unknown[] } }
    return (body.data?.reports ?? []).length
  }

  async waitForNewReport(projectId: string, previousReportId: string | null, timeoutMs = 60_000): Promise<void> {
    const start = Date.now()
    while (Date.now() - start < timeoutMs) {
      const latestId = await this.getLatestReportId(projectId)
      if (latestId && latestId !== previousReportId) {
        const count = await this.getReportCount(projectId)
        console.log(`  Report ready for project "${projectId}" (${count} reports)`)
        return
      }
      await new Promise((r) => setTimeout(r, 2_000))
    }
    throw new Error(`Timed out waiting for new report in project "${projectId}"`)
  }

  async waitForPlaywrightReport(projectId: string, reportId: string, timeoutMs = 30_000): Promise<boolean> {
    const start = Date.now()
    while (Date.now() - start < timeoutMs) {
      const res = await fetch(
        `${API_URL}/projects/${encodeURIComponent(projectId)}/playwright-reports/${reportId}/index.html`,
        { headers: this.authHeaders() },
      )
      if (res.ok) return true
      await new Promise((r) => setTimeout(r, 1_000))
    }
    return false
  }

  static tarGzDirectory(dirPath: string, outputPath: string): void {
    execSync(`tar czf "${outputPath}" -C "${dirPath}" .`, { stdio: 'pipe' })
  }

  /** Tar only files matching the given glob patterns (e.g. ['*.json']) — strips large attachments. */
  static tarGzDirectoryFiltered(dirPath: string, outputPath: string, includePatterns: string[]): void {
    const nameArgs = includePatterns.map((p) => `-name '${p}'`).join(' -o ')
    execSync(`cd "${dirPath}" && find . -maxdepth 1 -type f \\( ${nameArgs} \\) -print0 | tar czf "${outputPath}" --null -T -`, { stdio: 'pipe' })
  }

  async createProject(slug: string, parentId?: number): Promise<{ project_id: number; slug: string; parent_id?: number }> {
    const res = await fetch(`${API_URL}/projects`, {
      method: 'POST',
      headers: { ...this.authHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify(parentId != null ? { id: slug, parent_id: parentId } : { id: slug }),
    })
    if (!res.ok) throw new Error(`Create project "${slug}" failed: ${res.status} ${await res.text()}`)
    const body = (await res.json()) as { data: { project_id: number; slug: string; parent_id?: number } }
    return body.data
  }

  async listProjects(): Promise<Array<{ project_id: number; slug: string; parent_id?: number | null }>> {
    const res = await fetch(`${API_URL}/projects?per_page=200`, { headers: this.authHeaders() })
    if (!res.ok) throw new Error(`List projects failed: ${res.status} ${await res.text()}`)
    const body = (await res.json()) as { data: Array<{ project_id: number; slug: string; parent_id?: number | null }> }
    return body.data
  }

  async deleteProject(projectId: string): Promise<void> {
    const res = await fetch(`${API_URL}/projects/${encodeURIComponent(projectId)}`, {
      method: 'DELETE',
      headers: this.authHeaders(),
    })
    if (!res.ok && res.status !== 404) {
      throw new Error(`Delete project "${projectId}" failed: ${res.status} ${await res.text()}`)
    }
  }

  getReportUrl(projectId: string, reportId = 'latest'): string {
    const baseUrl = process.env.ALLUREDECK_URL ?? 'http://localhost:7474'
    return `${baseUrl}/projects/${encodeURIComponent(projectId)}/reports/${reportId}`
  }
}
