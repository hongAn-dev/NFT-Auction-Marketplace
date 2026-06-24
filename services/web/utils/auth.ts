export interface UserProfile {
  id: string;
  email: string;
  role: string;
}

const ACCESS_TOKEN_KEY = 'curatorial_access_token';
const REFRESH_TOKEN_KEY = 'curatorial_refresh_token';
const USER_PROFILE_KEY = 'curatorial_user_profile';

export function saveTokens(accessToken: string, refreshToken: string) {
  if (typeof window === 'undefined') return;
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
  
  // Decode JWT payload
  try {
    const payloadBase64 = accessToken.split('.')[1];
    const decodedPayload = JSON.parse(atob(payloadBase64));
    const profile: UserProfile = {
      id: decodedPayload.sub,
      email: decodedPayload.email,
      role: decodedPayload.role
    };
    localStorage.setItem(USER_PROFILE_KEY, JSON.stringify(profile));
  } catch (err) {
    console.error('Failed to parse JWT token payload:', err);
  }
}

export function clearTokens() {
  if (typeof window === 'undefined') return;
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(USER_PROFILE_KEY);
}

export function getAccessToken(): string | null {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken(): string | null {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function getUserProfile(): UserProfile | null {
  if (typeof window === 'undefined') return null;
  const profile = localStorage.getItem(USER_PROFILE_KEY);
  if (!profile) return null;
  try {
    return JSON.parse(profile) as UserProfile;
  } catch {
    return null;
  }
}

export function isAuthenticated(): boolean {
  return !!getAccessToken();
}

export async function logoutUser(apiGatewayUrl: string = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:4000'): Promise<boolean> {
  const token = getAccessToken();
  clearTokens();
  
  if (token) {
    try {
      await fetch(`${apiGatewayUrl}/api/auth/logout`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
    } catch (err) {
      console.warn('Network request for logout failed, tokens cleared locally:', err);
    }
  }
  return true;
}
