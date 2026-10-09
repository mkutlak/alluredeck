import { render, screen } from '@testing-library/react'
import { BarChart } from 'recharts'
import { ChartContainer } from '../chart'

// Recharts makes the chart's svg focusable for its keyboard tooltip; the container
// hid its outline, so the base focus ring never showed and a keyboard user could not
// see where focus was.
it('leaves the chart surface its focus ring', () => {
  render(
    <ChartContainer config={{}}>
      <BarChart data={[]} />
    </ChartContainer>,
  )
  expect(screen.getByTestId('chart-container').className).not.toContain(
    '[&_.recharts-surface]:outline-none',
  )
})
