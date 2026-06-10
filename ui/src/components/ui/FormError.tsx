interface FormErrorProps {
  /** Error message to display. When falsy the component renders nothing. */
  message?: string
}

/**
 * Renders an accessible inline error message with `role="alert"`.
 * Renders nothing when `message` is empty/undefined.
 *
 * Usage:
 *   <FormError message={error} />
 */
export function FormError({ message }: FormErrorProps) {
  if (!message) return null
  return (
    <p role="alert" className="text-destructive text-sm">
      {message}
    </p>
  )
}
