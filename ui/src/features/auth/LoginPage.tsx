import { useEffect, useState } from 'react'
import { useNavigate, useLocation, useSearchParams } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { login, getSession } from '@/api/auth'
import { getConfig } from '@/api/system'
import { extractErrorMessage } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import type { Role } from '@/store/auth'
import { env } from '@/lib/env'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'

export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const setAuth = useAuthStore((s) => s.setAuth)
  const raw = (location.state as { from?: Location })?.from?.pathname
  const from = raw?.startsWith('/') && !raw.startsWith('//') ? raw : '/'

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [errorMsg, setErrorMsg] = useState('')

  const { data: configData } = useQuery({
    queryKey: ['config'],
    queryFn: getConfig,
    staleTime: 5 * 60 * 1000,
  })

  const oidcEnabled = configData?.data.oidc_enabled ?? false

  // Handle OIDC callback: ?oidc=success
  useEffect(() => {
    if (searchParams.get('oidc') !== 'success') return

    let cancelled = false
    getSession()
      .then((res) => {
        if (cancelled) return
        const { username: user, roles, expires_in, provider } = res.data
        setAuth(roles as Role[], user, expires_in, provider)
        navigate(from, { replace: true })
      })
      .catch((err) => {
        if (cancelled) return
        setErrorMsg(extractErrorMessage(err))
      })

    return () => {
      cancelled = true
    }
  }, [searchParams, setAuth, navigate, from])

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

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setErrorMsg('')
    if (!username || !password) {
      setErrorMsg('Username and password are required.')
      return
    }
    loginMutation.mutate({ username, password })
  }

  const ssoLoginUrl = `${env.apiUrl}/auth/oidc/login`

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
          {oidcEnabled && (
            <div className="mb-4 space-y-4">
              <Button asChild variant="outline" className="w-full">
                <a href={ssoLoginUrl}>Sign in with SSO</a>
              </Button>
              <div className="relative">
                <div className="absolute inset-0 flex items-center">
                  <Separator className="w-full" />
                </div>
                <div className="relative flex justify-center text-xs uppercase">
                  <span className="bg-card text-muted-foreground px-2">or</span>
                </div>
              </div>
            </div>
          )}
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
