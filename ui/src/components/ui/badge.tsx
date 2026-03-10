import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-hidden focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground hover:bg-primary/80',
        secondary:
          'border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/80',
        destructive:
          'border-transparent bg-destructive text-destructive-foreground hover:bg-destructive/80',
        outline: 'text-foreground',
        passed:
          'border-transparent bg-[#40a02b]/15 text-[#40a02b] dark:bg-[#a6e3a1]/15 dark:text-[#a6e3a1]',
        failed:
          'border-transparent bg-[#d20f39]/15 text-[#d20f39] dark:bg-[#f38ba8]/15 dark:text-[#f38ba8]',
        broken:
          'border-transparent bg-[#fe640b]/15 text-[#fe640b] dark:bg-[#fab387]/15 dark:text-[#fab387]',
        skipped:
          'border-transparent bg-[#8c8fa1]/15 text-[#6c6f85] dark:bg-[#7f849c]/15 dark:text-[#a6adc8]',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />
}

// eslint-disable-next-line react-refresh/only-export-components -- shadcn pattern: variant helper co-located with component
export { Badge, badgeVariants }
