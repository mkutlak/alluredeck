import { NavLink } from 'react-router'
import { cn } from '@/lib/utils'

export interface TabsNavItem {
  to: string
  label: string
  end?: boolean
  'data-testid'?: string
}

export interface TabsNavProps {
  items: TabsNavItem[]
  'aria-label': string
}

export function TabsNav({ items, 'aria-label': ariaLabel }: TabsNavProps) {
  return (
    <nav aria-label={ariaLabel} className="flex items-center gap-1 border-b">
      {items.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.end}
          data-testid={item['data-testid']}
          className={({ isActive }) =>
            cn(
              '-mb-px border-b-2 px-3 py-2 text-sm transition-colors',
              isActive
                ? 'border-primary text-foreground font-medium'
                : 'text-muted-foreground hover:text-foreground hover:border-muted-foreground/30 border-transparent',
            )
          }
        >
          {item.label}
        </NavLink>
      ))}
    </nav>
  )
}
