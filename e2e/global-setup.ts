import { AllureDeckClient } from './fixtures/alluredeck-client'
import { USERNAME, PASSWORD } from './fixtures/credentials'

export default async function globalSetup() {
  console.log('\n[global-setup] Verifying AllureDeck auth...')

  const client = new AllureDeckClient()
  await client.login(USERNAME, PASSWORD)
  console.log(`  Authenticated as ${USERNAME}`)

  console.log('[global-setup] Ready.\n')
}
