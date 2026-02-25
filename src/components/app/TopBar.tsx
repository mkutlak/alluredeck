import { Link, useNavigate } from 'react-router-dom'
import { Moon, Sun, LogOut, User } from 'lucide-react'
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
import { useAuthStore } from '@/store/auth'
import { logout } from '@/api/auth'
import { toast } from '@/components/ui/use-toast'
import { env } from '@/lib/env'
import { ProjectSelector } from './ProjectSelector'

export function TopBar() {
  const { theme, setTheme } = useTheme()
  const { username, isAdmin, clearAuth } = useAuthStore()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

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
    <header className="flex h-14 shrink-0 items-center gap-4 border-b bg-background px-4">
      {/* Logo / brand */}
      <Link to="/" className="flex items-center gap-2 font-semibold">
        <img src="/favicon.svg" alt="Allure" className="h-5 w-5" />
        <span className="text-sm">{env.appTitle}</span>
      </Link>

      <ProjectSelector />

      <div className="flex-1" />

      {/* Theme toggle */}
      <Button
        variant="ghost"
        size="icon"
        onClick={handleThemeToggle}
        aria-label="Toggle theme"
      >
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
              <p className="text-xs text-muted-foreground">
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
