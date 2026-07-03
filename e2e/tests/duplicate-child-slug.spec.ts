import { test, expect } from '../fixtures/auth'
import { AllureDeckClient } from '../fixtures/alluredeck-client'
import { USERNAME, PASSWORD } from '../fixtures/credentials'

interface CreatedProject {
  slug: string
  projectId: number
}

interface ScenarioProjects {
  parentA: CreatedProject
  parentB: CreatedProject
  childUnderA: CreatedProject
  childUnderB: CreatedProject
  childSlug: string
}

async function setupScenario(): Promise<ScenarioProjects> {
  const client = new AllureDeckClient()
  await client.login(USERNAME, PASSWORD)

  const stamp = Date.now()
  const parentASlug = `e2e-parent-a-${stamp}`
  const parentBSlug = `e2e-parent-b-${stamp}`
  const childSlug = `child-x-${stamp}`

  const parentA = await client.createProject(parentASlug)
  const parentB = await client.createProject(parentBSlug)
  const childUnderA = await client.createProject(childSlug, parentA.project_id)
  const childUnderB = await client.createProject(childSlug, parentB.project_id)

  return {
    parentA: { slug: parentA.slug, projectId: parentA.project_id },
    parentB: { slug: parentB.slug, projectId: parentB.project_id },
    childUnderA: { slug: childUnderA.slug, projectId: childUnderA.project_id },
    childUnderB: { slug: childUnderB.slug, projectId: childUnderB.project_id },
    childSlug,
  }
}

async function cleanupScenario(scenario: ScenarioProjects): Promise<void> {
  const client = new AllureDeckClient()
  await client.login(USERNAME, PASSWORD)
  // Delete children first, then parents — endpoint takes numeric project_id.
  for (const id of [
    scenario.childUnderA.projectId,
    scenario.childUnderB.projectId,
    scenario.parentA.projectId,
    scenario.parentB.projectId,
  ]) {
    await client.deleteProject(String(id))
  }
}

test.describe('Duplicate child slug across different parents', () => {
  let scenario: ScenarioProjects

  test.beforeAll(async () => {
    scenario = await setupScenario()
  })

  test.afterAll(async () => {
    if (scenario) await cleanupScenario(scenario)
  })

  test('backend assigns distinct project_ids to same-slug children under different parents', () => {
    expect(scenario.childUnderA.slug).toBe(scenario.childSlug)
    expect(scenario.childUnderB.slug).toBe(scenario.childSlug)
    expect(scenario.childUnderA.projectId).not.toBe(scenario.childUnderB.projectId)
  })

  test('child overview header shows hierarchical label parentA/childSlug', async ({
    authenticatedPage: page,
  }) => {
    await page.goto(`/projects/${scenario.childUnderA.projectId}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('heading', { name: `${scenario.parentA.slug}/${scenario.childSlug}` }),
    ).toBeVisible()
  })

  test('child overview header shows hierarchical label parentB/childSlug', async ({
    authenticatedPage: page,
  }) => {
    await page.goto(`/projects/${scenario.childUnderB.projectId}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('heading', { name: `${scenario.parentB.slug}/${scenario.childSlug}` }),
    ).toBeVisible()
  })

  // The top-bar project switcher was removed in the IA redesign (spec:
  // docs/superpowers/specs/2026-07-02-alluredeck-ia-redesign-design.md).
  // Same-slug disambiguation in the UI is covered by the two hierarchical
  // header-label tests above.

  test('parent link from child overview navigates to the correct parent', async ({
    authenticatedPage: page,
  }) => {
    await page.goto(`/projects/${scenario.childUnderA.projectId}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })

    // Post-IA-redesign the parent link lives in the ProjectLayout page header
    // ("Part of: <parent>"), outside the project-overview container; the
    // breadcrumb also links the parent, so use the dedicated testid.
    await page.getByTestId('project-parent-link').click()
    await page.waitForURL(new RegExp(`/projects/${scenario.parentA.projectId}`), {
      timeout: 10_000,
    })
  })
})
