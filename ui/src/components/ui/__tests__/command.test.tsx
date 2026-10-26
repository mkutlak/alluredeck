import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  Command,
  CommandDialog,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '../command'

const FACT_HEADING = '[&_[cmdk-group-heading]]:text-fact'
const MUTED_HEADING = '[&_[cmdk-group-heading]]:text-muted-foreground'

// A group heading is a label for facts, not de-emphasised chrome: it reads in the fact ink.
describe('Command group headings', () => {
  it('colours them in the fact ink on a Command group', () => {
    render(
      <Command>
        <CommandList>
          <CommandGroup heading="Projects" data-testid="group">
            <CommandItem>api-cloud</CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>,
    )

    const group = screen.getByTestId('group')
    expect(group).toHaveClass(FACT_HEADING)
    expect(group).not.toHaveClass(MUTED_HEADING)
  })

  it('colours them in the fact ink in a CommandDialog', () => {
    render(
      <CommandDialog open>
        <CommandInput placeholder="Search" />
        <CommandList>
          <CommandGroup heading="Projects">
            <CommandItem>api-cloud</CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>,
    )

    const root = screen.getByRole('combobox').closest('[cmdk-root]')
    expect(root).toHaveClass(FACT_HEADING)
    expect(root).not.toHaveClass(MUTED_HEADING)
  })
})
