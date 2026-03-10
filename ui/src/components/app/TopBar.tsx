import { Link, useNavigate } from 'react-router'
import { Moon, Sun, LogOut, User, Search } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { useAuthStore } from '@/store/auth'
import { logout } from '@/api/auth'
import { toast } from '@/components/ui/use-toast'
import { useSearchCommand } from '@/features/search'
import { ProjectSwitcher } from './ProjectSwitcher'
import { CreateMenu } from './CreateMenu'

export function TopBar() {
  const { theme, setTheme } = useTheme()
  const username = useAuthStore((s) => s.username)
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { setOpen: setSearchOpen } = useSearchCommand()

  const logoutMutation = useMutation({
    mutationFn: logout,
    onSettled: () => {
      clearAuth()
      queryClient.clear()
      navigate('/login', { replace: true })
    },
    onError: () => {
      // Still clear local state even if server logout fails
      clearAuth()
      queryClient.clear()
      navigate('/login', { replace: true })
    },
  })

  const handleThemeToggle = () => {
    setTheme(theme === 'dark' ? 'light' : 'dark')
  }

  return (
    <header className="bg-background z-50 flex h-12 shrink-0 items-center gap-2 border-b px-4">
      <SidebarTrigger className="-ml-1" />
      <Separator orientation="vertical" className="h-4" />

      {/* Favicon only — no app title text */}
      <Link to="/">
        <img src="/favicon.svg" alt="Allure" className="h-5 w-5" />
      </Link>
      <Separator orientation="vertical" className="h-4" />

      {/* Project switcher */}
      <ProjectSwitcher />

      <div className="flex-1" />

      {/* Search trigger */}
      <Button
        variant="ghost"
        className="text-muted-foreground h-8 gap-2 px-3 text-sm"
        onClick={() => setSearchOpen(true)}
        aria-label="Search"
      >
        <Search size={16} />
        <span className="hidden sm:inline">Search...</span>
        <kbd className="bg-muted pointer-events-none hidden rounded border px-1.5 py-0.5 font-mono text-[10px] font-medium select-none sm:inline">
          ⌘K
        </kbd>
      </Button>

      {/* Create menu (admin only) */}
      <CreateMenu />

      {/* Theme toggle */}
      <Button variant="ghost" size="icon" onClick={handleThemeToggle} aria-label="Toggle theme">
        {theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
      </Button>

      {/* User menu */}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" aria-label="User menu">
            <User size={16} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuLabel className="font-normal">
            <div className="flex flex-col space-y-1">
              <p className="text-sm font-medium">{username ?? 'User'}</p>
              <p className="text-muted-foreground text-xs">
                {isAdmin() ? 'Administrator' : 'Viewer'}
              </p>
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => {
              logoutMutation.mutate()
              toast({ title: 'Signed out', description: 'You have been logged out.' })
            }}
            className="text-destructive focus:text-destructive"
          >
            <LogOut size={14} />
            Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </header>
  )
}
