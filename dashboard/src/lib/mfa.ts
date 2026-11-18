// MFA code entry. The server accepts, in the same `totp_code` field, either a
// 6-digit authenticator code or a one-time backup code. Backup codes are
// base32 (A-Z, 2-7) formatted XXXX-XXXX, and the server compares a hash of the
// exact string, so the UI must send them uppercased with the dash.

const BACKUP_CODE = /^[A-Z2-7]{4}-[A-Z2-7]{4}$/;

// normalizeMfaInput cleans what the user typed or pasted. For backup codes it
// uppercases, drops anything outside the base32 alphabet (spaces, the dash),
// and re-inserts the dash after four characters.
export function normalizeMfaInput(raw: string, backup: boolean): string {
  if (!backup) return raw.replace(/\D/g, "").slice(0, 6);
  const chars = raw.toUpperCase().replace(/[^A-Z2-7]/g, "").slice(0, 8);
  return chars.length > 4 ? `${chars.slice(0, 4)}-${chars.slice(4)}` : chars;
}

export function isCompleteMfaCode(code: string, backup: boolean): boolean {
  return backup ? BACKUP_CODE.test(code) : /^\d{6}$/.test(code);
}
