import { useState, useEffect, useRef } from 'react';
import { GetServerSideProps } from 'next';
import Head from 'next/head';
import Link from 'next/link';
import { io, Socket } from 'socket.io-client';
import { ArrowLeft, Clock, Shield, Award, Database, FileText, Lock } from 'lucide-react';
import { getUserProfile, getAccessToken, UserProfile } from '../../utils/auth';
import { useToast } from '../../context/ToastContext';

interface NFT {
  id: string;
  title: string;
  description: string;
  image_url: string;
  creator_id: string;
  owner_id: string;
  start_price: number;
  status: string;
}

interface NFTMetadata {
  rarity: string;
  attributes: Record<string, any>;
  ipfs_hash: string;
  created_by: string;
}

interface HighestBid {
  bidId: string;
  nftId: string;
  userId: string;
  amount: number;
  createdAt: string;
}

interface DetailProps {
  nft: NFT;
  metadata: NFTMetadata | null;
  initialHighestBid: HighestBid | null;
  error?: string;
}

export default function NFTDetail({ nft, metadata, initialHighestBid, error }: DetailProps) {
  const { showToast } = useToast();
  const [highestBid, setHighestBid] = useState<HighestBid | null>(initialHighestBid);
  const [bids, setBids] = useState<HighestBid[]>([]);
  const [bidAmount, setBidAmount] = useState<string>('');
  const [loading, setLoading] = useState<boolean>(false);
  const [timeLeft, setTimeLeft] = useState<string>('');
  const [user, setUser] = useState<UserProfile | null>(null);
  const [isClient, setIsClient] = useState(false);
  const [balance, setBalance] = useState<number | null>(null);
  const [ethBalance, setEthBalance] = useState<string | null>(null);
  const socketRef = useRef<Socket | null>(null);

  const formatVND = (amountInUSD: number) => {
    return `${(amountInUSD * 25000).toLocaleString('vi-VN')} VND`;
  };

  const formatETH = (amountInUSD: number) => {
    return `${(amountInUSD / 3000).toFixed(4)} ETH`;
  };

  const formatCurrency = (amountInUSD: number) => {
    return `${formatVND(amountInUSD)} (~${formatETH(amountInUSD)})`;
  };

  const handleBidAmountChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    if (user && !user.email.endsWith('@web3.auth')) {
      const clean = value.replace(/\D/g, '');
      if (clean === '') {
        setBidAmount('');
      } else {
        const num = parseInt(clean, 10);
        setBidAmount(num.toLocaleString('vi-VN'));
      }
    } else {
      const clean = value.replace(/[^0-9.]/g, '');
      const parts = clean.split('.');
      if (parts.length > 2) return;
      setBidAmount(clean);
    }
  };

  useEffect(() => {
    setIsClient(true);
    const profile = getUserProfile();
    setUser(profile);

    if (profile) {
      const email = profile.email || '';
      const isWeb3 = email.endsWith('@web3.auth');
      if (isWeb3) {
        const address = email.split('@')[0];
        if (address.startsWith('0x') && typeof window !== 'undefined' && (window as any).ethereum) {
          const fetchEthBalance = async () => {
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
            }
          };
          fetchEthBalance();
        }
      } else {
        const fetchWeb2Balance = async () => {
          const token = getAccessToken();
          if (!token) return;
          try {
            const res = await fetch('http://localhost:4000/api/auth/payment/balance', {
              headers: { Authorization: `Bearer ${token}` }
            });
            const data = await res.json();
            if (res.ok && data.success) {
              setBalance(data.data.balance);
            }
          } catch (err) {
            console.error('Failed to fetch Web2 balance:', err);
          }
        };
        fetchWeb2Balance();
      }
    }
  }, []);

  // Countdown timer simulation
  useEffect(() => {
    const end = new Date();
    end.setHours(end.getHours() + 24); // 24 hours countdown from loading the page

    const interval = setInterval(() => {
      const now = new Date().getTime();
      const diff = end.getTime() - now;

      if (diff <= 0) {
        clearInterval(interval);
        setTimeLeft('AUCTION CLOSED');
      } else {
        const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
        const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
        const seconds = Math.floor((diff % (1000 * 60)) / 1000);
        setTimeLeft(`${hours.toString().padStart(2, '0')}h ${minutes.toString().padStart(2, '0')}m ${seconds.toString().padStart(2, '0')}s`);
      }
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  // WebSockets Realtime bidding updates
  useEffect(() => {
    // Connect to NestJS BFF Gateway
    const socket = io('http://localhost:4000', {
      transports: ['websocket'],
      forceNew: true
    });

    socketRef.current = socket;

    socket.on('connect', () => {
      console.log('🔌 Connected to WebSocket BFF Gateway! Socket ID:', socket.id);
      
      // Join Room specifically for this NFT
      socket.emit('joinRoom', { nftId: nft.id }, (ack: any) => {
        console.log('📥 Joined Room ACK:', ack);
      });
    });

    // Receive live bid updates
    socket.on('bid:updated', (data: HighestBid) => {
      console.log('⚡ Received Live Bid Update Event:', data);
      if (data.nftId === nft.id) {
        setHighestBid(data);
        setBids((prev) => [data, ...prev].slice(0, 10)); // Keep last 10 bids
        
        // Show live notification toast
        showToast(
          `New live bid placed: ${user && user.email.endsWith('@web3.auth') ? formatETH(data.amount) : formatVND(data.amount)} by collector ${data.userId.substring(0, 8)}...`,
          'info'
        );
      }
    });

    socket.on('connect_error', (err) => {
      console.error('⚠️ WebSocket Gateway connection error:', err);
    });

    return () => {
      if (socket) {
        socket.emit('leaveRoom', { nftId: nft.id });
        socket.disconnect();
      }
    };
  }, [nft.id]);

  const handlePlaceBid = async (e: React.FormEvent) => {
    e.preventDefault();
    const isWeb3 = user && user.email.endsWith('@web3.auth');
    const rawAmount = isWeb3 
      ? bidAmount.replace(/,/g, '') 
      : bidAmount.replace(/\./g, '').replace(/,/g, '');

    if (!rawAmount || isNaN(parseFloat(rawAmount))) {
      showToast('Please enter a valid numeric bidding amount.', 'error');
      return;
    }

    const currentLimit = highestBid ? highestBid.amount : nft.start_price;
    const inputVal = parseFloat(rawAmount);
    
    // Convert client-entered bid value to USD base currency
    let bidValInUSD = 0;
    if (isWeb3) {
      bidValInUSD = inputVal * 3000;
    } else {
      bidValInUSD = inputVal / 25000;
    }

    if (bidValInUSD <= currentLimit) {
      showToast(
        `Your bid must be strictly greater than current value of ${user && user.email.endsWith('@web3.auth') ? formatETH(currentLimit) : formatVND(currentLimit)}`,
        'error'
      );
      return;
    }

    // Client-side balance check
    if (user) {
      if (isWeb3) {
        if (ethBalance === null) {
          showToast('Still fetching your ETH wallet balance, please try again.', 'error');
          return;
        }
        if (parseFloat(ethBalance) < inputVal) {
          showToast(`Insufficient ETH balance. Connected wallet has ${ethBalance} ETH.`, 'error');
          return;
        }
      } else {
        if (balance === null) {
          showToast('Still fetching your account balance, please try again.', 'error');
          return;
        }
        if (balance < inputVal) {
          showToast(`Insufficient balance. Your current balance is ${balance.toLocaleString('vi-VN')} VND.`, 'error');
          return;
        }
      }
    }

    setLoading(true);

    try {
      const token = getAccessToken();
      if (!token) {
        throw new Error('Authentication is required to place a bid. Please log in.');
      }

      const res = await fetch('http://localhost:4000/api/v1/bids', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          nftId: nft.id,
          amount: bidValInUSD
        })
      });

      const data = await res.json();
      if (!res.ok || !data.success) {
        throw new Error(data.error?.message || 'Bidding request rejected by system validation.');
      }

      showToast('Your bid has been processed and broadcasted successfully!', 'success');
      setBidAmount('');
      // Update local balance state immediately
      if (user) {
        if (isWeb3) {
          const email = user.email || '';
          const address = email.split('@')[0];
          if (address.startsWith('0x') && typeof window !== 'undefined' && (window as any).ethereum) {
            try {
              const hexBalance = await (window as any).ethereum.request({
                method: 'eth_getBalance',
                params: [address, 'latest'],
              });
              const wei = BigInt(hexBalance);
              const eth = (Number(wei) / 1e18).toFixed(4);
              setEthBalance(eth);
            } catch (err) {
              console.error('Failed to update ETH balance:', err);
            }
          }
        } else {
          setBalance((prev) => (prev !== null ? prev - inputVal : null));
        }
      }
    } catch (err: any) {
      showToast(err.message || 'System failed to register bid. Ensure all docker containers are active.', 'error');
    } finally {
      setLoading(false);
    }
  };

  const currentPrice = highestBid ? highestBid.amount : nft.start_price;

  return (
    <div>
      <Head>
        <title>{nft.title.toUpperCase()} — CURATORIAL</title>
      </Head>

      <div style={{ marginBottom: '2rem' }}>
        <Link href="/" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.5rem', fontWeight: 700, textTransform: 'uppercase', fontSize: '0.8rem', letterSpacing: '0.05em' }}>
          <ArrowLeft size={16} /> BACK TO EXHIBITION
        </Link>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.2fr) minmax(0, 1fr)', gap: '4rem', alignItems: 'start' }}>
        {/* Left Column: Media Gallery */}
        <div>
          <div className="card" style={{ borderWidth: '2px', position: 'relative' }}>
            <div className="card-image-container" style={{ paddingBottom: '100%' }}>
              <img
                src={nft.image_url || 'https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?q=80&w=600&auto=format&fit=crop'}
                alt={nft.title}
                className="card-image"
              />
            </div>
          </div>

          {/* MongoDB Unstructured Metadata Block */}
          <div style={{ marginTop: '2.5rem' }}>
            <h4 style={{ fontSize: '1rem', borderBottom: '1px solid #000', paddingBottom: '0.5rem', marginBottom: '1rem', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <Database size={16} /> Dynamic MongoDB Metadata (Fault Tolerant)
            </h4>

            {metadata ? (
              <div className="border-box" style={{ padding: '1.5rem', background: '#FFFFFF' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.2rem', borderBottom: '1px solid #EAE8E0', paddingBottom: '0.8rem' }}>
                  <span style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#666', fontWeight: 700 }}>Curator Rarity Grade</span>
                  <span className="badge badge-live" style={{ padding: '0.4rem 0.8rem', fontSize: '0.75rem' }}>
                    <Award size={12} style={{ display: 'inline', marginRight: '0.3rem' }} /> {metadata.rarity.toUpperCase()}
                  </span>
                </div>

                <div style={{ marginBottom: '1.2rem' }}>
                  <span style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#666', fontWeight: 700, display: 'block', marginBottom: '0.4rem' }}>IPFS Digital Certificate</span>
                  <div style={{ fontFamily: 'monospace', background: '#FAF9F6', padding: '0.6rem 0.8rem', fontSize: '0.75rem', border: '1px solid #EAE8E0', overflowWrap: 'anywhere' }}>
                    <FileText size={12} style={{ display: 'inline', marginRight: '0.4rem' }} /> {metadata.ipfs_hash}
                  </div>
                </div>

                {metadata.attributes && Object.keys(metadata.attributes).length > 0 && (
                  <div>
                    <span style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#666', fontWeight: 700, display: 'block', marginBottom: '0.5rem' }}>Generative Traits</span>
                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '0.8rem' }}>
                      {Object.entries(metadata.attributes).map(([key, value]) => (
                        <div key={key} style={{ background: '#FAF9F6', border: '1px solid #EAE8E0', padding: '0.6rem 0.8rem' }}>
                          <span style={{ fontSize: '0.65rem', textTransform: 'uppercase', color: '#777', display: 'block' }}>{key}</span>
                          <span style={{ fontSize: '0.85rem', fontWeight: 700 }}>{String(value)}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="border-box" style={{ background: '#FAF9F6', color: '#666', padding: '1.5rem', fontSize: '0.85rem', lineHeight: '1.5' }}>
                <p>No unstructured dynamic metadata is loaded for this artwork. You can add dynamic properties to this NFT by calling the HTTP Upsert endpoint `/api/v1/nfts/:id/metadata` on the bidding service.</p>
              </div>
            )}
          </div>
        </div>

        {/* Right Column: Catalog details & Bidding Action */}
        <div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', marginBottom: '1.5rem' }}>
            <span style={{ fontSize: '0.8rem', color: 'var(--accent-color)', fontWeight: 800, textTransform: 'uppercase', letterSpacing: '0.08em' }}>
              Vernissage Lot #{nft.id.substring(0, 8).toUpperCase()}
            </span>
            <h1 style={{ fontSize: '3rem', fontWeight: 800, lineHeight: 1.05 }}>{nft.title}</h1>
            <div style={{ display: 'flex', gap: '2rem', marginTop: '0.5rem' }}>
              <div>
                <span style={{ fontSize: '0.7rem', textTransform: 'uppercase', color: '#666', letterSpacing: '0.05em' }}>Artist</span>
                <div style={{ fontWeight: 700, fontSize: '0.9rem' }}>{nft.creator_id}</div>
              </div>
              <div>
                <span style={{ fontSize: '0.7rem', textTransform: 'uppercase', color: '#666', letterSpacing: '0.05em' }}>Owner</span>
                <div style={{ fontWeight: 700, fontSize: '0.9rem' }}>{nft.owner_id}</div>
              </div>
            </div>
          </div>

          <div style={{ borderBottom: '1px solid #000', paddingBottom: '2rem', marginBottom: '2rem' }}>
            <p style={{ color: '#555', fontSize: '1.05rem', lineHeight: 1.6 }}>{nft.description || 'No description available for this lot.'}</p>
          </div>

          {/* Active Auction Bidding Panel */}
          <div className="border-box" style={{ borderWidth: '2px', background: '#FFFFFF', padding: '2rem', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <span style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#666', fontWeight: 700, letterSpacing: '0.05em' }}>Value Valuation</span>
                {user && user.email.endsWith('@web3.auth') ? (
                  <div>
                    <h2 style={{ fontSize: '2.5rem', color: 'var(--accent-color)', fontWeight: 800, marginTop: '0.2rem', lineHeight: '1.2' }}>
                      {formatETH(currentPrice)}
                    </h2>
                    <div style={{ fontSize: '0.95rem', color: '#666', fontWeight: 700, marginTop: '0.25rem' }}>
                      ≈ {formatVND(currentPrice)}
                    </div>
                  </div>
                ) : (
                  <div>
                    <h2 style={{ fontSize: '2.5rem', color: 'var(--accent-color)', fontWeight: 800, marginTop: '0.2rem', lineHeight: '1.2' }}>
                      {formatVND(currentPrice)}
                    </h2>
                    <div style={{ fontSize: '0.95rem', color: '#666', fontWeight: 700, marginTop: '0.25rem' }}>
                      ≈ {formatETH(currentPrice)}
                    </div>
                  </div>
                )}
              </div>
              <div style={{ textAlign: 'right' }}>
                <span style={{ fontSize: '0.75rem', textTransform: 'uppercase', color: '#666', fontWeight: 700, letterSpacing: '0.05em', display: 'flex', alignItems: 'center', gap: '0.3rem', justifyContent: 'flex-end' }}>
                  <Clock size={12} /> Time Remaining
                </span>
                <div style={{ fontFamily: 'var(--font-headline)', fontWeight: 800, fontSize: '1.2rem', marginTop: '0.3rem' }}>
                  {timeLeft || '00h 00m 00s'}
                </div>
              </div>
            </div>

            {!isClient ? (
              <div style={{ textAlign: 'center', padding: '1rem', color: '#666' }}>Loading curatorial bidding modules...</div>
            ) : !user ? (
              <div style={{ borderTop: '1px solid #EAE8E0', paddingTop: '1.5rem', textAlign: 'center', display: 'flex', flexDirection: 'column', gap: '1rem', alignItems: 'center' }}>
                <Lock size={32} className="text-accent" />
                <div style={{ fontWeight: 800, fontSize: '0.9rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  Collector Authentication Required
                </div>
                <p style={{ fontSize: '0.8rem', color: '#666', lineHeight: 1.4 }}>
                  You must sign in with your collector signature to place a binding bid offer.
                </p>
                <Link href="/auth" className="btn btn-accent" style={{ width: '100%', padding: '0.8rem' }}>
                  LOG IN / REGISTER
                </Link>
              </div>
            ) : (
              <form onSubmit={handlePlaceBid} style={{ borderTop: '1px solid #EAE8E0', paddingTop: '1.5rem' }}>
                <div className="form-group" style={{ marginBottom: '1rem' }}>
                  <label className="form-label">COLLECTOR IDENTITY</label>
                  <input
                    type="text"
                    className="form-input"
                    value={`${user.email} (${user.role.toUpperCase()})`}
                    disabled
                    style={{ background: '#FAF9F6', color: '#666', cursor: 'not-allowed' }}
                  />
                </div>

                <div className="form-group" style={{ marginBottom: '1.5rem' }}>
                  <label className="form-label" htmlFor="bid-amount">
                    BID OFFER ({user.email.endsWith('@web3.auth') ? 'ETH' : 'VND'})
                  </label>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                    <input
                      id="bid-amount"
                      type="text"
                      className="form-input"
                      style={{ fontSize: '1.5rem', fontWeight: 700, padding: '0.6rem 1rem' }}
                      value={bidAmount}
                      onChange={handleBidAmountChange}
                      placeholder={
                        user.email.endsWith('@web3.auth')
                          ? (currentPrice / 3000 + 0.01).toFixed(4)
                          : (currentPrice * 25000 + 10000).toLocaleString('vi-VN')
                      }
                      required
                    />
                    {user.email.endsWith('@web3.auth') ? (
                      <span style={{ fontSize: '0.75rem', color: '#666' }}>
                        Your Wallet Balance: <strong>{ethBalance !== null ? `${ethBalance} ETH` : 'Loading...'}</strong>
                      </span>
                    ) : (
                      <span style={{ fontSize: '0.75rem', color: '#666' }}>
                        Your Account Balance: <strong>{balance !== null ? `${balance.toLocaleString('vi-VN')} VND` : 'Loading...'}</strong>
                      </span>
                    )}
                  </div>
                </div>

                <button type="submit" disabled={loading} className="btn btn-accent" style={{ width: '100%', fontSize: '1rem', padding: '1rem' }}>
                  {loading ? 'TRANSMITTING BID OFFER...' : 'SUBMIT AUTHORIZED BID'}
                </button>
              </form>
            )}

            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', background: '#FAF9F6', padding: '0.8rem 1rem', fontSize: '0.75rem', color: '#666' }}>
              <Shield size={16} className="text-accent" />
              <span>Bids are binding, validated through gRPC and protected against race conditions.</span>
            </div>
          </div>

          {/* Live Activity Feed */}
          <div style={{ marginTop: '2.5rem' }}>
            <h4 style={{ fontSize: '0.9rem', borderBottom: '1px solid #000', paddingBottom: '0.5rem', marginBottom: '1rem', letterSpacing: '0.05em' }}>
              LIVE TRANSACTION FEED
            </h4>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.8rem' }}>
              {bids.length === 0 ? (
                <div style={{ padding: '1.5rem', background: '#FAF9F6', border: '1px dashed #CCC', textAlign: 'center', fontSize: '0.8rem', color: '#666' }}>
                  Awaiting first collector offer. All WebSocket events will stream here live.
                </div>
              ) : (
                bids.map((b) => (
                  <div key={b.bidId} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: '#FFFFFF', border: '1px solid #EAE8E0', padding: '0.8rem 1.2rem', animation: 'flash 1s' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.8rem' }}>
                      <div style={{ width: '8px', height: '8px', background: 'var(--accent-color)' }}></div>
                      <div>
                        <div style={{ fontWeight: 700, fontSize: '0.85rem' }}>{b.userId}</div>
                        <div style={{ fontSize: '0.65rem', color: '#999' }}>{new Date(b.createdAt).toLocaleTimeString()}</div>
                      </div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ fontWeight: 800, fontSize: '1.1rem', color: 'var(--accent-color)' }}>
                        {user && user.email.endsWith('@web3.auth') ? formatETH(b.amount) : formatVND(b.amount)}
                      </div>
                      <div style={{ fontSize: '0.75rem', color: '#666', fontWeight: 600, marginTop: '0.1rem' }}>
                        ≈ {user && user.email.endsWith('@web3.auth') ? formatVND(b.amount) : formatETH(b.amount)}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export const getServerSideProps: GetServerSideProps = async (context) => {
  const { id } = context.params as { id: string };
  let nft: NFT | null = null;
  let metadata: NFTMetadata | null = null;
  let initialHighestBid: HighestBid | null = null;
  let errorMsg = '';

  const backendUrls = [
    `http://bidding-service:8080/api/v1/nfts/${id}`,
    `http://localhost:8080/api/v1/nfts/${id}`
  ];

  for (const url of backendUrls) {
    try {
      const controller = new AbortController();
      const abortId = setTimeout(() => controller.abort(), 2000);
      
      const res = await fetch(url, { signal: controller.signal });
      clearTimeout(abortId);
      
      if (res.ok) {
        const body = await res.json();
        if (body && body.success) {
          nft = body.data;
          metadata = body.metadata || null;
          break;
        }
      }
    } catch (e: any) {
      errorMsg = e.message;
    }
  }

  // If NFT fetched successfully, get the highest bid from api-gateway BFF
  if (nft) {
    const gatewayUrls = [
      `http://api-gateway:4000/api/v1/nfts/${id}/highest-bid`,
      `http://localhost:4000/api/v1/nfts/${id}/highest-bid`
    ];

    for (const url of gatewayUrls) {
      try {
        const controller = new AbortController();
        const abortId = setTimeout(() => controller.abort(), 1500);
        
        const res = await fetch(url, { signal: controller.signal });
        clearTimeout(abortId);
        
        if (res.ok) {
          const body = await res.json();
          if (body && body.success && body.data) {
            initialHighestBid = body.data;
            break;
          }
        }
      } catch (e) {
        // Silent catch, fallback is fine
      }
    }
  }

  // Nếu không tìm thấy NFT thực tế trong CSDL, trả về trang 404 tiêu chuẩn
  if (!nft) {
    return {
      notFound: true
    };
  }

  return {
    props: {
      nft,
      metadata,
      initialHighestBid
    }
  };
};
