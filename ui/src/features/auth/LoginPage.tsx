import { useState } from 'react'
import { useNavigate, useLocation } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { login } from '@/api/auth'
import { extractErrorMessage } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import type { Role } from '@/store/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const setAuth = useAuthStore((s) => s.setAuth)
  const from = (location.state as { from?: Location })?.from?.pathname ?? '/'

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [errorMsg, setErrorMsg] = useState('')

  const loginMutation = useMutation({
    mutationFn: login,
    onSuccess: (res) => {
      const { roles, expires_in } = res.data
      setAuth(roles as Role[], username, expires_in)
      navigate(from, { replace: true })
    },
    onError: (err) => {
      setErrorMsg(extractErrorMessage(err))
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setErrorMsg('')
    if (!username || !password) {
      setErrorMsg('Username and password are required.')
      return
    }
    loginMutation.mutate({ username, password })
  }

  return (
    <div className="bg-muted/30 flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="space-y-1 text-center">
          <div className="mb-2 flex justify-center">
            <img src="/favicon.svg" alt="Allure" className="h-10 w-10" />
          </div>
          <CardTitle className="text-2xl">AllureDeck</CardTitle>
          <CardDescription>Sign in to your account</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                type="text"
                autoComplete="username"
                placeholder="admin"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                disabled={loginMutation.isPending}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                placeholder="••••••••"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={loginMutation.isPending}
              />
            </div>

            {errorMsg && (
              <p role="alert" className="text-destructive text-sm">
                {errorMsg}
              </p>
            )}

            <Button type="submit" className="w-full" disabled={loginMutation.isPending}>
              {loginMutation.isPending ? (
                <>
                  <Loader2 className="animate-spin" />
                  Signing in…
                </>
              ) : (
                'Sign in'
              )}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
