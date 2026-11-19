// Never display upstream error messages: they may contain login credentials.
// Only recognized reasons and our own stage markers become translated advice.
export function twoFAImportFailureKey(cause: unknown): string | undefined {
  const error = cause as { message?: unknown; reason?: unknown; response?: { data?: { reason?: unknown } } } | null
  const reason = error?.reason ?? error?.response?.data?.reason
  const message = typeof error?.message === 'string' ? error.message : ''
  if (reason === 'AUTO_CONFIG_GROUPS_REQUIRED' || reason === 'GROUP_NOT_FOUND' ||
      message === 'import_groups_required' || /reason="(?:AUTO_CONFIG_GROUPS_REQUIRED|GROUP_NOT_FOUND)"/.test(message)) {
    return 'openaiRelogin.twoFA.failures.groupsRequired'
  }
  if (message === 'account_import_failed') return 'openaiRelogin.twoFA.failures.accountImport'
  if (message === 'credential_enrollment_failed') return 'openaiRelogin.twoFA.failures.credentialEnrollment'
  return undefined
}
