import { GetServerSideProps } from 'next';
import Link from 'next/link';
import Image from 'next/image';
import { ArrowRight, Sparkles, AlertCircle } from 'lucide-react';

interface NFT {
  id: string;
  title: string;
  description: string;
  image_url: string;
  creator_id: string;
  owner_id: string;
  start_price: number;
  status: string;
  created_at?: number;
}

interface IndexProps {
  nfts: NFT[];
  error?: string;
}

export default function Home({ nfts, error }: IndexProps) {
  return (
    <div>
      {/* Magazine Style Hero Banner */}
      <section className="hero-banner" style={{ borderBottom: '2px solid #000', paddingBottom: '4rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '2rem' }}>
          <div>
            <div className="badge badge-live" style={{ marginBottom: '1.5rem', display: 'inline-flex', alignItems: 'center', gap: '0.4rem' }}>
              <Sparkles size={12} /> Active Vernissage
            </div>
            <h1 className="hero-title" style={{ fontSize: '4.5rem', margin: '0 0 1rem 0', fontWeight: 800, letterSpacing: '-0.04em' }}>
              MONOCHROME & FORM
            </h1>
            <p className="hero-subtitle" style={{ fontSize: '1.1rem', color: '#666', maxWidth: '650px', lineHeight: 1.6 }}>
              A curated collection of exceptional digital artworks highlighting structure, raw textures, and sharp geometry. Built on modern blockchain technology with Cloudflare R2 high-speed direct media storage.
            </p>
          </div>
          <div className="border-box" style={{ maxWidth: '350px', background: '#FFFFFF', padding: '1.5rem', borderColor: '#000' }}>
            <h3 style={{ fontSize: '0.9rem', marginBottom: '0.8rem', letterSpacing: '0.05em' }}>Gallery Notice</h3>
            <p style={{ fontSize: '0.85rem', color: '#555', marginBottom: '1rem', lineHeight: '1.4' }}>
              All digital works are fully certified, with dynamic metadata stored securely on decentralized databases and verified on-chain.
            </p>
            <Link href="/create" className="btn btn-secondary" style={{ width: '100%', fontSize: '0.75rem', padding: '0.6rem' }}>
              Submit Artwork <ArrowRight size={14} />
            </Link>
          </div>
        </div>
      </section>

      {/* Exhibitions Section */}
      <section style={{ margin: '4rem 0' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: '2.5rem', borderBottom: '1px solid #000', paddingBottom: '1rem' }}>
          <h2 style={{ fontSize: '1.8rem', fontWeight: 800 }}>CURRENT EXHIBITION</h2>
          <span style={{ fontSize: '0.85rem', fontWeight: 600, color: '#666', fontFamily: 'var(--font-headline)' }}>
            {nfts.length} ARTWORKS AVAILABLE
          </span>
        </div>

        {error && (
          <div className="border-box" style={{ borderColor: 'var(--accent-color)', background: '#FFF5F5', display: 'flex', gap: '1rem', alignItems: 'center', marginBottom: '2rem' }}>
            <AlertCircle className="text-accent" />
            <div>
              <strong style={{ display: 'block', fontSize: '0.9rem' }}>Storage or Connection Warning</strong>
              <span style={{ fontSize: '0.85rem', color: '#666' }}>{error}. Displaying mock items for demonstration.</span>
            </div>
          </div>
        )}

        {nfts.length === 0 ? (
          <div className="border-box" style={{ textAlign: 'center', padding: '5rem 2rem' }}>
            <h3 style={{ marginBottom: '1rem' }}>No Artworks Found</h3>
            <p style={{ color: '#666', marginBottom: '2rem' }}>Be the first to submit a premium digital artwork to our curated catalog.</p>
            <Link href="/create" className="btn">
              Submit Artwork Now
            </Link>
          </div>
        ) : (
          <div className="gallery-grid">
            {nfts.map((nft) => (
              <article key={nft.id} className="card">
                <Link href={`/nft/${nft.id}`}>
                  <div className="card-image-container">
                    <img
                      src={nft.image_url || 'https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?q=80&w=600&auto=format&fit=crop'}
                      alt={nft.title}
                      className="card-image"
                      loading="lazy"
                    />
                    <div style={{ position: 'absolute', top: '1rem', right: '1rem' }}>
                      <span className={`badge ${nft.status === 'active' ? 'badge-live' : ''}`}>
                        {nft.status || 'Active'}
                      </span>
                    </div>
                  </div>
                </Link>
                <div className="card-body">
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                    <div>
                      <span style={{ fontSize: '0.7rem', color: '#777', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '0.2rem' }}>
                        ID: {nft.id.substring(0, 12)}...
                      </span>
                      <h3 className="card-title" style={{ fontSize: '1.2rem', fontWeight: 800 }}>
                        <Link href={`/nft/${nft.id}`}>{nft.title}</Link>
                      </h3>
                    </div>
                  </div>
                  <p className="card-desc">{nft.description || 'Curated digital asset with fully automated bidding pipeline and instant asset settlement.'}</p>
                  
                  <div className="card-meta">
                    <div>
                      <div className="meta-label">Artist / Creator</div>
                      <div style={{ fontSize: '0.85rem', fontWeight: 600 }}>{nft.creator_id}</div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div className="meta-label">Current Value</div>
                      <div className="meta-value" style={{ color: 'var(--accent-color)' }}>
                        {nft.start_price.toLocaleString('en-US', { style: 'currency', currency: 'USD' })}
                      </div>
                    </div>
                  </div>

                  <Link href={`/nft/${nft.id}`} className="btn btn-secondary" style={{ width: '100%', marginTop: '0.5rem', textAlign: 'center', justifyContent: 'center' }}>
                    ENTER GALLERY <ArrowRight size={14} />
                  </Link>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

export const getServerSideProps: GetServerSideProps = async () => {
  let nfts: NFT[] = [];
  let errorMsg = '';

  const backendUrls = [
    'http://bidding-service:8080/api/v1/nfts',  // Docker internal DNS
    'http://localhost:8080/api/v1/nfts'        // Host machine fallback
  ];

  for (const url of backendUrls) {
    try {
      const controller = new AbortController();
      const id = setTimeout(() => controller.abort(), 2000);
      
      const res = await fetch(url, { signal: controller.signal });
      clearTimeout(id);
      
      if (res.ok) {
        const body = await res.json();
        if (body && body.success && Array.isArray(body.data)) {
          nfts = body.data;
          break;
        }
      }
    } catch (e: any) {
      errorMsg = e.message;
    }
  }

  // Nếu không tải được NFT từ các backend, trả về danh sách rỗng và thông báo lỗi
  if (nfts.length === 0 && errorMsg !== "") {
    return {
      props: {
        nfts: [],
        error: `Could not reach backend services (${errorMsg || 'Connection timed out'}).`
      }
    };
  }

  return {
    props: {
      nfts
    }
  };
};
