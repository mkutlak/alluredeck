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

  test('project switcher dropdown disambiguates same-slug children', async ({
    authenticatedPage: page,
  }) => {
    await page.goto(`/projects/${scenario.childUnderA.projectId}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })

    // Trigger button shows the active child as parentA/childSlug
    const trigger = page.getByRole('button', {
      name: `${scenario.parentA.slug}/${scenario.childSlug}`,
    })
    await expect(trigger).toBeVisible()
    await trigger.click()

    // Both same-slug children appear with disambiguating parent prefix in the
    // "All Projects" group. Scope to that group: the active child is also
    // echoed under "Recents", which would make a bare option lookup ambiguous.
    const allProjects = page.getByRole('group', { name: 'All Projects' })
    await expect(
      allProjects.getByRole('option', { name: `${scenario.parentA.slug}/${scenario.childSlug}` }),
    ).toBeVisible()
    await expect(
      allProjects.getByRole('option', { name: `${scenario.parentB.slug}/${scenario.childSlug}` }),
    ).toBeVisible()
  })

  test('parent link from child overview navigates to the correct parent', async ({
    authenticatedPage: page,
  }) => {
    await page.goto(`/projects/${scenario.childUnderA.projectId}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })

    // The parent is linked from both the breadcrumb and the overview body;
    // scope to the overview so the locator resolves to a single link.
    await page
      .getByTestId('project-overview')
      .getByRole('link', { name: scenario.parentA.slug })
      .click()
    await page.waitForURL(new RegExp(`/projects/${scenario.parentA.projectId}`), {
      timeout: 10_000,
    })
  })
})
