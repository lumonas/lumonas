// WebAuthn ceremony helpers. The daemon speaks go-webauthn's JSON dialect,
// which matches the WebAuthn spec: challenges and ids are base64url, and the
// finish endpoints consume the browser Credential serialized as JSON.

import { apiPost } from '@/api/client'

function base64urlToBuffer(value: string): ArrayBuffer {
  const padding = '='.repeat((4 - (value.length % 4)) % 4)
  const base64 = (value + padding).replace(/-/g, '+').replace(/_/g, '/')
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

function bufferToBase64url(value: ArrayBuffer): string {
  const bytes = new Uint8Array(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function passkeysSupported(): boolean {
  return typeof window !== 'undefined' && window.isSecureContext === true && typeof window.PublicKeyCredential !== 'undefined'
}


// JSON shapes returned by the daemon (base64url strings, not DOM buffers).
interface CreationOptionsJson {
  challenge: string
  rp: { id?: string; name?: string }
  user: { id: string; name?: string; displayName?: string }
  pubKeyCredParams: { type: 'public-key'; alg: number }[]
  timeout?: number
  excludeCredentials?: { id: string; type?: string }[]
  authenticatorSelection?: {
    residentKey?: ResidentKeyRequirement
    userVerification?: UserVerificationRequirement
  }
  attestation?: AttestationConveyancePreference
}

interface RequestOptionsJson {
  challenge: string
  rpId?: string
  timeout?: number
  userVerification?: UserVerificationRequirement
  allowCredentials?: { id: string; type?: 'public-key' }[]
}

export interface Passkey {
  id: string
  name: string
  createdAt: string
}

export async function registerPasskey(userName: string, name?: string): Promise<void> {
  const optionsEnvelope = await apiPost<{ publicKey: CreationOptionsJson }>('/users/self/passkeys/register/begin')
  const options = optionsEnvelope.publicKey
  const publicKey = {
      challenge: base64urlToBuffer(options.challenge),
      rp: options.rp,
      user: {
        id: base64urlToBuffer(options.user.id),
        name: options.user.name ?? userName,
        displayName: options.user.displayName ?? userName,
      },
      pubKeyCredParams: options.pubKeyCredParams,
      timeout: options.timeout,
      excludeCredentials: (options.excludeCredentials ?? []).map((c: { id: string }) => ({
        id: base64urlToBuffer(c.id),
        type: 'public-key' as const,
      })),
    authenticatorSelection: options.authenticatorSelection,
    attestation: options.attestation,
  }
  const credential = await navigator.credentials.create({ publicKey: publicKey as unknown as PublicKeyCredentialCreationOptions }) as PublicKeyCredential
  await apiPost<{ ok: boolean }>(
    `/users/self/passkeys/register/finish${name ? `?name=${encodeURIComponent(name)}` : ''}`,
    {
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        clientDataJSON: bufferToBase64url((credential.response as AuthenticatorAttestationResponse).clientDataJSON),
        attestationObject: bufferToBase64url((credential.response as AuthenticatorAttestationResponse).attestationObject),
        transports: (credential.response as AuthenticatorAttestationResponse).getTransports?.() ?? [],
      },
    },
  )
}

export async function signInWithPasskey(username: string): Promise<{ csrfToken: string; username: string }> {
  const begin = await apiPost<{ publicKey: RequestOptionsJson }>('/auth/passkeys/login/begin', { username })
  const options = begin.publicKey
  const publicKey = {
    challenge: base64urlToBuffer(options.challenge),
    rpId: options.rpId,
    timeout: options.timeout,
    userVerification: options.userVerification,
    allowCredentials: (options.allowCredentials ?? []).map((c: { id: string }) => ({
      id: base64urlToBuffer(c.id),
        type: 'public-key' as const,
    })),
  }
  const credential = await navigator.credentials.get({ publicKey: publicKey as unknown as PublicKeyCredentialRequestOptions }) as PublicKeyCredential
  return apiPost<{ csrfToken: string; username: string }>('/auth/passkeys/login/finish', {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: bufferToBase64url((credential.response as AuthenticatorAssertionResponse).clientDataJSON),
      authenticatorData: bufferToBase64url((credential.response as AuthenticatorAssertionResponse).authenticatorData),
      signature: bufferToBase64url((credential.response as AuthenticatorAssertionResponse).signature),
      userHandle: (credential.response as AuthenticatorAssertionResponse).userHandle
        ? bufferToBase64url((credential.response as AuthenticatorAssertionResponse).userHandle as ArrayBuffer)
        : null,
    },
  })
}
