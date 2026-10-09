import { createPortal } from 'react-dom'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { navigateRows } from '../rowNavigation'

// Rows as the feed draws them: the branch link is the row's target, a row without a
// branch link marks its toggle instead, and the third row has neither. Row two is open:
// its panel holds a nested list and a stray data-row-link, as AI-summary markdown can.
function Feed() {
  return (
    <>
      <button type="button">outside</button>
      <ul onKeyDown={navigateRows}>
        <li>
          <button type="button">toggle one</button>
          <a href="#one" data-row-link="">
            one
          </a>
          <a href="#chip-one">chip one</a>
        </li>
        <li>
          <button type="button" data-row-link="">
            toggle two
          </button>
          <input aria-label="in-list input" />
          <textarea aria-label="in-list textarea" />
          <select aria-label="in-list select">
            <option>a</option>
          </select>
          <div role="combobox" tabIndex={0} aria-label="in-list combobox" aria-expanded="false" />
          <div role="textbox" contentEditable tabIndex={0} aria-label="in-list editor" />
          <div>
            <ul>
              <li>
                <a href="#evidence">evidence link</a>
              </li>
            </ul>
            <a href="#stray" data-row-link="">
              stray
            </a>
          </div>
        </li>
        <li>no target</li>
        <li>
          <a href="#three" data-row-link="">
            three
          </a>
          <a href="#chip-three">chip three</a>
          {createPortal(<button type="button">portaled</button>, document.body)}
        </li>
      </ul>
    </>
  )
}

const el = (name: string) => screen.getByRole('link', { name })
const control = (name: string) => screen.getByRole('button', { name })

describe('navigateRows', () => {
  it('does nothing while focus is outside the list', async () => {
    const user = userEvent.setup()
    render(<Feed />)

    await user.keyboard('j')
    expect(document.body).toHaveFocus()
    control('outside').focus()
    await user.keyboard('j')
    expect(control('outside')).toHaveFocus()
  })

  // The toggle stands in for a row without a branch link, so j does not skip it;
  // a row with no target at all is passed over, and both ends hold.
  it('moves row to row with j and k, and stays at the ends', async () => {
    const user = userEvent.setup()
    render(<Feed />)
    el('one').focus()

    await user.keyboard('k')
    expect(el('one')).toHaveFocus()
    await user.keyboard('j')
    expect(control('toggle two')).toHaveFocus()
    await user.keyboard('j')
    expect(el('three')).toHaveFocus()
    await user.keyboard('j')
    expect(el('three')).toHaveFocus()
    await user.keyboard('kk')
    expect(el('one')).toHaveFocus()
  })

  it.each([
    { from: () => control('toggle one'), key: 'j', to: () => control('toggle two') },
    { from: () => el('evidence link'), key: 'j', to: () => el('three') },
    { from: () => el('evidence link'), key: 'k', to: () => el('one') },
    // The row's own target, never a link that its content happens to mark.
    { from: () => el('three'), key: 'k', to: () => control('toggle two') },
    { from: () => el('chip one'), key: 'j', to: () => control('toggle two') },
    { from: () => el('chip three'), key: 'k', to: () => control('toggle two') },
  ])(
    'goes from anywhere in a row to the next or previous row ($key)',
    async ({ from, key, to }) => {
      const user = userEvent.setup()
      render(<Feed />)
      from().focus()

      await user.keyboard(key)
      expect(to()).toHaveFocus()
    },
  )

  // Caps Lock gives "J" without Shift; Shift+J is someone else's shortcut.
  it('reads J and K without Shift as j and k', async () => {
    const user = userEvent.setup()
    render(<Feed />)
    el('one').focus()

    await user.keyboard('J')
    expect(control('toggle two')).toHaveFocus()
    await user.keyboard('K')
    expect(el('one')).toHaveFocus()
  })

  it.each([
    { name: 'Shift', keys: '{Shift>}j{/Shift}' },
    { name: 'Control', keys: '{Control>}j{/Control}' },
    { name: 'Meta', keys: '{Meta>}j{/Meta}' },
    { name: 'Alt', keys: '{Alt>}j{/Alt}' },
  ])('leaves j alone with $name held', async ({ keys }) => {
    const user = userEvent.setup()
    render(<Feed />)
    el('one').focus()

    await user.keyboard(keys)
    expect(el('one')).toHaveFocus()
  })

  it.each([
    { name: 'an input', target: () => screen.getByRole('textbox', { name: 'in-list input' }) },
    { name: 'a textarea', target: () => screen.getByRole('textbox', { name: 'in-list textarea' }) },
    { name: 'a select', target: () => screen.getByRole('combobox', { name: 'in-list select' }) },
    {
      name: 'a combobox',
      target: () => screen.getByRole('combobox', { name: 'in-list combobox' }),
    },
    {
      name: 'an editable region',
      target: () => screen.getByRole('textbox', { name: 'in-list editor' }),
    },
    // A portal inside a row (a tooltip, a popover) bubbles through React, not the DOM.
    { name: 'a portal', target: () => control('portaled') },
  ])('leaves j alone in $name', async ({ target }) => {
    const user = userEvent.setup()
    render(<Feed />)
    target().focus()

    await user.keyboard('j')
    expect(target()).toHaveFocus()
  })

  // Consumed at the ends too, or Firefox's quick find would take the j.
  it.each([
    { name: 'j on the last row', from: () => el('three'), key: 'j' },
    { name: 'k on the first row', from: () => el('one'), key: 'k' },
  ])('consumes $name', ({ from, key }) => {
    render(<Feed />)
    expect(fireEvent.keyDown(from(), { key })).toBe(false)
  })

  it.each([
    { name: 'outside the list', from: () => control('outside'), init: {} },
    { name: 'from a portal', from: () => control('portaled'), init: {} },
    {
      name: 'in an input',
      from: () => screen.getByRole('textbox', { name: 'in-list input' }),
      init: {},
    },
    { name: 'with Control held', from: () => el('one'), init: { ctrlKey: true } },
  ])('leaves j unconsumed $name', ({ from, init }) => {
    render(<Feed />)
    expect(fireEvent.keyDown(from(), { key: 'j', ...init })).toBe(true)
  })
})
