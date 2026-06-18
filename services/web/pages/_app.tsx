import type { AppProps } from 'next/app';
import Head from 'next/head';
import Link from 'next/link';
import { useRouter } from 'next/router';
import { useState, useEffect } from 'react';
import { Wallet } from 'lucide-react';
import { getUserProfile, logoutUser, UserProfile } from '../utils/auth';
import { ToastProvider } from '../context/ToastContext';
import '../styles/global.css';

const formatUserIdentifier = (email: string) => {
  const name = email.split('@')[0];
  if (name.startsWith('0x') && name.length === 42) {
    return `${name.substring(0, 6)}...${name.substring(38)}`;
  }
  if (name.length > 15) {
    return `${name.substring(0, 12)}...`;
  }
  return name;
};

export default function App({ Component, pageProps }: AppProps) {
  const router = useRouter();
  const [user, setUser] = useState<UserProfile | null>(null);
  const [ethBalance, setEthBalance] = useState<string | null>(null);

  // Sync auth state on mount and on route change
  useEffect(() => {
    setUser(getUserProfile());
    
    const handleRouteChange = () => {
      setUser(getUserProfile());
    };
    
    router.events.on('routeChangeComplete', handleRouteChange);
    return () => {
      router.events.off('routeChangeComplete', handleRouteChange);
    };
  }, [router]);

  // Fetch ETH balance for Web3 users
  useEffect(() => {
    const fetchBalance = async () => {
      if (!user) {
        setEthBalance(null);
        return;
      }
      const email = user.email || '';
      const address = email.split('@')[0];
      const isWeb3User = email.endsWith('@web3.auth') && address.startsWith('0x');

      if (isWeb3User && typeof window !== 'undefined' && (window as any).ethereum) {
        try {
          const hexBalance = await (window as any).ethereum.request({
            method: 'eth_getBalance',
            params: [address, 'latest'],
          });
          const wei = BigInt(hexBalance);
          const eth = (Number(wei) / 1e18).toFixed(4);
          setEthBalance(eth);
        } catch (err) {
          console.error('Failed to fetch Web3 balance:', err);
          setEthBalance(null);
        }
      } else {
        setEthBalance(null);
      }
    };

    fetchBalance();
  }, [user]);

  const handleLogout = async () => {
    await logoutUser();
    setUser(null);
    router.push('/auth');
  };

  return (
    <ToastProvider>
      <Head>
        <title>CURATORIAL — Premium NFT Auction & Marketplace</title>
        <meta name="description" content="Institutional Curatorial Minimalist NFT Auction and Marketplace. Discover exceptional digital artworks in a state-of-the-art gallery space." />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" href="/favicon.ico" />
      </Head>

      <div className="layout">
        <header className="header">
          <div className="header-container">
            <Link href="/" className="logo">
              CURATORIAL<span className="logo-dot"></span>
            </Link>
            <nav className="nav">
              <Link href="/" className={`nav-link ${router.pathname === '/' ? 'active' : ''}`}>
                Exhibitions
              </Link>
              <Link href="/create" className={`nav-link ${router.pathname === '/create' ? 'active' : ''}`}>
                Submit Artwork
              </Link>
            </nav>
            <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
              {user ? (
                <>
                  <Link href="/wallet" style={{ display: 'flex', alignItems: 'center', color: '#000000', padding: '0.5rem', border: '1px solid #000000', background: '#FAF9F6', cursor: 'pointer' }} title="Wallet & Balance">
                    <Wallet size={16} />
                  </Link>
                  <span style={{ fontSize: '0.75rem', fontWeight: 700, letterSpacing: '0.05em', textTransform: 'uppercase', border: '1px solid #000', padding: '0.5rem 1rem', background: '#FFFFFF' }}>
                    {formatUserIdentifier(user.email)} ({user.role})
                  </span>
                  <button 
                    onClick={handleLogout}
                    style={{ 
                      fontSize: '0.75rem', 
                      fontWeight: 700, 
                      letterSpacing: '0.05em', 
                      textTransform: 'uppercase', 
                      background: 'var(--accent-color)', 
                      color: '#FFFFFF',
                      border: '1px solid var(--accent-color)', 
                      padding: '0.5rem 1rem',
                      cursor: 'pointer'
                    }}
                  >
                    LOG OUT
                  </button>
                </>
              ) : (
                <>
                  <Link 
                    href="/auth?wallet=true"
                    style={{ 
                      fontSize: '0.75rem', 
                      fontWeight: 700, 
                      letterSpacing: '0.05em', 
                      textTransform: 'uppercase', 
                      border: '1px solid #000', 
                      padding: '0.5rem 1rem',
                      background: '#FFFFFF',
                      color: '#000000',
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      gap: '0.5rem'
                    }}
                  >
                    <svg width="14" height="14" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M29.56 12.18l-3.32-5.75c-.86-1.5-2.73-2.1-4.22-1.25L16 8.71 9.98 5.18C8.49 4.33 6.62 4.93 5.76 6.43L2.44 12.18c-.86 1.5-.32 3.42 1.22 4.22l12.34 6.42 12.34-6.42c1.54-.8 2.08-2.72 1.22-4.22z" fill="#E2761B"/>
                      <path d="M16 26.67L5.76 16.4l1.22-2.11 9.02 5.25 9.02-5.25 1.22 2.11L16 26.67z" fill="#E2761B"/>
                    </svg>
                    CONNECT WALLET
                  </Link>
                  <Link 
                    href="/auth"
                    style={{ 
                      fontSize: '0.75rem', 
                      fontWeight: 700, 
                      letterSpacing: '0.05em', 
                      textTransform: 'uppercase', 
                      border: '1px solid #000', 
                      padding: '0.5rem 1rem',
                      background: '#000000',
                      color: '#FFFFFF'
                    }}
                  >
                    LOG IN / REGISTER
                  </Link>
                </>
              )}
            </div>
          </div>
        </header>

        <main className="main-content">
          <Component {...pageProps} />
        </main>

        <footer className="footer">
          <div className="footer-container">
            <div>© {new Date().getFullYear()} CURATORIAL GALLERY. ALL RIGHTS RESERVED.</div>
            <div style={{ display: 'flex', gap: '1.5rem' }}>
              <a href="#" style={{ textDecoration: 'underline' }}>Terms</a>
              <a href="#" style={{ textDecoration: 'underline' }}>Privacy</a>
              <a href="#" style={{ textDecoration: 'underline' }}>S3/R2 Storage Active</a>
            </div>
          </div>
        </footer>
      </div>

    </ToastProvider>
  );
}
