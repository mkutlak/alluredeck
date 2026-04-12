import { AllureDeckClient } from './fixtures/alluredeck-client'

export default async function globalSetup() {
  console.log('\n[global-setup] Verifying AllureDeck auth...')

  const client = new AllureDeckClient()
  await client.login('admin', 'admin')
  console.log('  Authenticated as admin')

  console.log('[global-setup] Ready.\n')
}
