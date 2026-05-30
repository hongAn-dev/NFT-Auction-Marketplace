import type { AppProps } from 'next/app';
import Head from 'next/head';
import Link from 'next/link';
import { useRouter } from 'next/router';
import '../styles/global.css';

export default function App({ Component, pageProps }: AppProps) {
  const router = useRouter();

  return (
    <>
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
            <div>
              <span style={{ fontSize: '0.75rem', fontWeight: 700, letterSpacing: '0.05em', textTransform: 'uppercase', border: '1px solid #000', padding: '0.5rem 1rem' }}>
                Collector Mode
              </span>
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
    </>
  );
}
