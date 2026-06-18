import { useState, useEffect } from 'react';
import Head from 'next/head';
import { useRouter } from 'next/router';
import { User, Key, Mail, ShieldAlert } from 'lucide-react';
import { saveTokens, isAuthenticated } from '../utils/auth';
import { useToast } from '../context/ToastContext';

export default function AuthPage() {
  const router = useRouter();
  const { showToast } = useToast();
  const [isLogin, setIsLogin] = useState(true);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [loading, setLoading] = useState(false);
  const [web3Loading, setWeb3Loading] = useState(false);
  const [web3Status, setWeb3Status] = useState('');

  // Redirect to home if already logged in
  useEffect(() => {
    if (isAuthenticated()) {
      router.push('/');
    }
  }, [router]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);

    const apiGatewayUrl = 'http://localhost:4000';
    const endpoint = isLogin ? '/api/auth/login' : '/api/auth/register';
    const payload = isLogin 
      ? { email, password } 
      : { email, password, name };

    try {
      const res = await fetch(`${apiGatewayUrl}${endpoint}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      const data = await res.json();
      if (!res.ok || data.success === false) {
        throw new Error(data.error?.message || data.message || 'Authentication request failed.');
      }

      if (isLogin) {
        // Save token and redirect
        saveTokens(data.data.accessToken, data.data.refreshToken);
        showToast('Authenticated successfully. Welcome collector!', 'success');
        setTimeout(() => {
          router.push('/');
        }, 1500);
      } else {
        showToast('Registration complete. Please log in.', 'success');
        setIsLogin(true);
        setPassword('');
      }
    } catch (err: any) {
      showToast(err.message || 'Network connection error.', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleWeb3SignIn = async () => {
    // 1. Check MetaMask installation
    const ethereum = (window as any).ethereum;
    if (!ethereum) {
      showToast('MetaMask extension not detected. Please install it to continue.', 'error');
      return;
    }

    setWeb3Loading(true);
    setWeb3Status('CONNECTING WALLET...');

    try {
      // 2. Request account access
      const accounts = await ethereum.request({ method: 'eth_requestAccounts' });
      if (!accounts || accounts.length === 0) {
        throw new Error('No accounts selected.');
      }
      const address = accounts[0];
      
      // 3. Request cryptographic Nonce from Backend
      setWeb3Status('FETCHING CRYPTO NONCE...');
      const apiGatewayUrl = 'http://localhost:4000';
      const nonceRes = await fetch(`${apiGatewayUrl}/api/auth/web3/nonce`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address })
      });
      
      const nonceData = await nonceRes.json();
      if (!nonceRes.ok || !nonceData.success) {
        throw new Error(nonceData.message || 'Failed to acquire auth challenge nonce.');
      }
      
      const { nonce, message } = nonceData.data;

      // 4. Request Signature using SIWE format
      setWeb3Status('AWAITING SIGNATURE...');
      const signature = await ethereum.request({
        method: 'personal_sign',
        params: [message, address]
      });

      // 5. Verify Signature with Backend
      setWeb3Status('VERIFYING AUTHENTICITY...');
      const verifyRes = await fetch(`${apiGatewayUrl}/api/auth/web3/verify`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address, signature })
      });

      const verifyData = await verifyRes.json();
      if (!verifyRes.ok || !verifyData.success) {
        throw new Error(verifyData.message || 'Signature verification rejected.');
      }

      // 6. Save Tokens and Redirect
      saveTokens(verifyData.data.accessToken, verifyData.data.refreshToken);
      showToast('Web3 Session authenticated successfully.', 'success');
      setTimeout(() => {
        router.push('/');
      }, 1500);

    } catch (err: any) {
      console.error(err);
      showToast(err.message || 'Web3 authentication failed.', 'error');
    } finally {
      setWeb3Loading(false);
      setWeb3Status('');
    }
  };

  // Auto-connect on query parameter redirect
  useEffect(() => {
    if (router.isReady && router.query.wallet === 'true' && !isAuthenticated()) {
      handleWeb3SignIn();
    }
  }, [router.isReady, router.query]);

  return (
    <div style={{ maxWidth: '450px', margin: '4rem auto' }}>
      <Head>
        <title>{isLogin ? 'LOG IN' : 'REGISTER'} — CURATORIAL</title>
      </Head>

      <div style={{ textAlign: 'center', marginBottom: '2.5rem' }}>
        <h1 style={{ fontSize: '2.5rem', fontWeight: 800, letterSpacing: '-0.04em' }}>
          {isLogin ? 'ACCESS GALLERY' : 'CREATE ACCOUNT'}
        </h1>
        <p style={{ color: '#666', fontSize: '0.85rem', marginTop: '0.5rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          {isLogin ? 'Enter credentials to bid and publish lots' : 'Register to participate in live digital exhibitions'}
        </p>
      </div>

      <div className="border-box" style={{ background: '#FFFFFF', borderWidth: '1px' }}>
        {/* Toggle tabs */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', borderBottom: '1px solid #000', margin: '-2rem -2rem 2rem -2rem' }}>
          <button 
            onClick={() => { setIsLogin(true); }}
            style={{
              background: isLogin ? 'transparent' : '#FAF9F6',
              border: 'none',
              borderRight: '1px solid #000',
              padding: '1rem',
              fontFamily: 'var(--font-headline)',
              fontWeight: 800,
              fontSize: '0.8rem',
              cursor: 'pointer',
              textTransform: 'uppercase',
              letterSpacing: '0.05em'
            }}
          >
            LOG IN
          </button>
          <button 
            onClick={() => { setIsLogin(false); }}
            style={{
              background: !isLogin ? 'transparent' : '#FAF9F6',
              border: 'none',
              padding: '1rem',
              fontFamily: 'var(--font-headline)',
              fontWeight: 800,
              fontSize: '0.8rem',
              cursor: 'pointer',
              textTransform: 'uppercase',
              letterSpacing: '0.05em'
            }}
          >
            REGISTER
          </button>
        </div>

        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '1.2rem' }}>
          {!isLogin && (
            <div className="form-group">
              <label className="form-label" htmlFor="reg-name">Full Collector Name</label>
              <div style={{ position: 'relative' }}>
                <User size={16} style={{ position: 'absolute', left: '1rem', top: '1rem', color: '#888' }} />
                <input
                  id="reg-name"
                  type="text"
                  className="form-input"
                  style={{ paddingLeft: '2.5rem', width: '100%' }}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. John Doe"
                  required
                />
              </div>
            </div>
          )}

          <div className="form-group">
            <label className="form-label" htmlFor="auth-email">Email Signature</label>
            <div style={{ position: 'relative' }}>
              <Mail size={16} style={{ position: 'absolute', left: '1rem', top: '1rem', color: '#888' }} />
              <input
                id="auth-email"
                type="email"
                className="form-input"
                style={{ paddingLeft: '2.5rem', width: '100%' }}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="collector@gallery.com"
                required
              />
            </div>
          </div>

          <div className="form-group">
            <label className="form-label" htmlFor="auth-password">Secret Phrase (Password)</label>
            <div style={{ position: 'relative' }}>
              <Key size={16} style={{ position: 'absolute', left: '1rem', top: '1rem', color: '#888' }} />
              <input
                id="auth-password"
                type="password"
                className="form-input"
                style={{ paddingLeft: '2.5rem', width: '100%' }}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
                required
              />
            </div>
          </div>

          <button 
            type="submit" 
            disabled={loading || web3Loading} 
            className="btn btn-accent" 
            style={{ width: '100%', padding: '1rem', marginTop: '0.5rem' }}
          >
            {loading ? 'TRANSMITTING CREDENTIALS...' : isLogin ? 'SIGN IN' : 'REGISTER PROFILE'}
          </button>
        </form>

        <div style={{ margin: '1.5rem 0', display: 'flex', alignItems: 'center', gap: '1rem' }}>
          <div style={{ flex: 1, height: '1px', background: '#DDD' }}></div>
          <span style={{ fontSize: '0.7rem', fontWeight: 700, color: '#888', letterSpacing: '0.1em' }}>OR AUTHENTICATE VIA</span>
          <div style={{ flex: 1, height: '1px', background: '#DDD' }}></div>
        </div>

        <button
          onClick={handleWeb3SignIn}
          disabled={loading || web3Loading}
          style={{
            width: '100%',
            padding: '1rem',
            background: '#000000',
            color: '#FFFFFF',
            border: '1px solid #000000',
            fontFamily: 'var(--font-headline)',
            fontWeight: 800,
            fontSize: '0.8rem',
            letterSpacing: '0.05em',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '0.75rem',
            transition: 'opacity 0.2s ease'
          }}
          onMouseEnter={(e) => e.currentTarget.style.opacity = '0.9'}
          onMouseLeave={(e) => e.currentTarget.style.opacity = '1'}
        >
          {web3Loading ? (
            web3Status
          ) : (
            <>
              <svg width="18" height="18" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M29.56 12.18l-3.32-5.75c-.86-1.5-2.73-2.1-4.22-1.25L16 8.71 9.98 5.18C8.49 4.33 6.62 4.93 5.76 6.43L2.44 12.18c-.86 1.5-.32 3.42 1.22 4.22l12.34 6.42 12.34-6.42c1.54-.8 2.08-2.72 1.22-4.22z" fill="#E2761B"/>
                <path d="M16 26.67L5.76 16.4l1.22-2.11 9.02 5.25 9.02-5.25 1.22 2.11L16 26.67z" fill="#E2761B"/>
              </svg>
              CONNECT WEB3 WALLET
            </>
          )}
        </button>
      </div>
    </div>
  );
}

