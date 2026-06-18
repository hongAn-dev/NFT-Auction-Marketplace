import { useState, useEffect } from 'react';
import Head from 'next/head';
import { useRouter } from 'next/router';
import { Wallet, PlusCircle, ArrowUpRight, History, CreditCard, X, Copy, Check } from 'lucide-react';
import { getAccessToken, isAuthenticated, getUserProfile } from '../utils/auth';
import { useToast } from '../context/ToastContext';

interface Transaction {
  id: string;
  amount: number;
  gatewayRefId: string;
  status: string;
  createdAt: string;
}

interface PaymentInfo {
  qrUrl: string;
  bankName: string;
  accountNo: string;
  accountName: string;
  memo: string;
  amount: number;
}

export default function WalletPage() {
  const router = useRouter();
  const { showToast } = useToast();
  const [balance, setBalance] = useState<number | null>(null);
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [depositAmount, setDepositAmount] = useState<string>('50000');
  const [loading, setLoading] = useState(false);
  const [fetchLoading, setFetchLoading] = useState(true);
  const [paymentInfo, setPaymentInfo] = useState<PaymentInfo | null>(null);
  const [copiedField, setCopiedField] = useState<string | null>(null);
  const [ethBalance, setEthBalance] = useState<string | null>(null);
  const [userProfile, setUserProfile] = useState<any | null>(null);

  const apiGatewayUrl = 'http://localhost:4000';

  useEffect(() => {
    if (!isAuthenticated()) {
      router.push('/auth');
      return;
    }

    const profile = getUserProfile();
    setUserProfile(profile);
    if (profile) {
      const email = profile.email || '';
      const address = email.split('@')[0];
      const isWeb3User = email.endsWith('@web3.auth') && address.startsWith('0x');
      if (isWeb3User && typeof window !== 'undefined' && (window as any).ethereum) {
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
    }

    fetchWalletData();
  }, []);

  // Polling wallet data when payment info is displayed
  useEffect(() => {
    let intervalId: NodeJS.Timeout;
    if (paymentInfo) {
      intervalId = setInterval(() => {
        fetchWalletData();
      }, 3000);
    }
    return () => {
      if (intervalId) clearInterval(intervalId);
    };
  }, [paymentInfo]);

  const fetchWalletData = async () => {
    const token = getAccessToken();
    if (!token) return;

    try {
      // 1. Fetch balance
      const balanceRes = await fetch(`${apiGatewayUrl}/api/auth/payment/balance`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      const balanceData = await balanceRes.json();
      if (balanceRes.ok && balanceData.success) {
        setBalance(balanceData.data.balance);
      }

      // 2. Fetch transactions
      const txRes = await fetch(`${apiGatewayUrl}/api/auth/payment/transactions`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      const txData = await txRes.json();
      if (txRes.ok && txData.success) {
        setTransactions(txData.data);

        // Check if current pending transaction succeeded
        if (paymentInfo) {
          const matchedTx = txData.data.find((tx: Transaction) => tx.gatewayRefId === paymentInfo.memo);
          if (matchedTx && matchedTx.status === 'SUCCESS') {
            showToast('Deposit successful! Your balance has been updated.', 'success');
            setPaymentInfo(null);
          }
        }
      }
    } catch (err) {
      console.error('Failed to load wallet data:', err);
    } finally {
      setFetchLoading(false);
    }
  };

  const handleDeposit = async (e: React.FormEvent) => {
    e.preventDefault();
    const amountNum = parseInt(depositAmount);
    if (isNaN(amountNum) || amountNum < 2000) {
      showToast('Minimum deposit amount is 2,000 VND', 'error');
      return;
    }

    setLoading(true);
    const token = getAccessToken();

    try {
      const res = await fetch(`${apiGatewayUrl}/api/auth/payment/deposit`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify({ amount: amountNum })
      });

      const data = await res.json();
      if (!res.ok || !data.success) {
        throw new Error(data.message || 'Could not initialize payment transaction.');
      }

      setPaymentInfo(data.data);
      showToast('VietQR payment details initialized!', 'success');

    } catch (err: any) {
      showToast(err.message || 'Connection error when depositing.', 'error');
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = (text: string, field: string) => {
    navigator.clipboard.writeText(text);
    setCopiedField(field);
    showToast(`Copied ${field} to clipboard`, 'success');
    setTimeout(() => setCopiedField(null), 2000);
  };

  const formatVND = (value: number) => {
    return new Intl.NumberFormat('vi-VN', { style: 'currency', currency: 'VND' }).format(value);
  };

  if (fetchLoading) {
    return (
      <div style={{ textAlign: 'center', margin: '6rem auto' }}>
        <p style={{ letterSpacing: '0.1em', fontWeight: 700, textTransform: 'uppercase' }}>LOADING WALLET DATA...</p>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: '900px', margin: '2rem auto', padding: '0 1rem' }}>
      <Head>
        <title>WALLET & BALANCE — CURATORIAL</title>
      </Head>

      <div style={{ borderBottom: '1px solid #000', paddingBottom: '1.5rem', marginBottom: '2.5rem' }}>
        <h1 style={{ fontSize: '2.5rem', fontWeight: 800, letterSpacing: '-0.04em' }}>WALLET & BALANCE</h1>
        <p style={{ color: '#666', fontSize: '0.8rem', textTransform: 'uppercase', letterSpacing: '0.05em', marginTop: '0.25rem' }}>
          Manage your local payment balance and SePay VietQR transaction history
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2rem' }}>
        {/* Left column: Balance info & Deposit form */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '2rem' }}>
          {/* Balance card */}
          <div className="border-box" style={{ background: '#FAF9F6', padding: '2rem', borderWidth: '1px', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem' }}>
                <Wallet size={20} />
                <span style={{ fontSize: '0.75rem', fontWeight: 700, letterSpacing: '0.1em', textTransform: 'uppercase', color: '#666' }}>
                  Current Balance (Credits)
                </span>
              </div>
              <div style={{ fontSize: '2.25rem', fontWeight: 800, fontFamily: 'var(--font-headline)' }}>
                {balance !== null ? formatVND(balance) : '0 ₫'}
              </div>
              <p style={{ fontSize: '0.75rem', color: '#888', marginTop: '0.25rem', fontStyle: 'italic' }}>
                * Used for bidding in local currency (VND).
              </p>
            </div>

            {ethBalance !== null && (
              <div style={{ borderTop: '1px solid #DDD', paddingTop: '1.2rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem' }}>
                  <svg width="18" height="18" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M16 2L6 18.5 16 24.5 26 18.5 16 2z" fill="#343434"/>
                    <path d="M16 24.5V30l10-11.5L16 24.5z" fill="#8C8C8C"/>
                    <path d="M16 30v-5.5L6 18.5 16 30z" fill="#3C3C3C"/>
                    <path d="M16 2v22.5L26 18.5 16 2z" fill="#141414"/>
                  </svg>
                  <span style={{ fontSize: '0.75rem', fontWeight: 700, letterSpacing: '0.1em', textTransform: 'uppercase', color: '#666' }}>
                    Connected Wallet (ETH)
                  </span>
                </div>
                <div style={{ fontSize: '1.8rem', fontWeight: 800, fontFamily: 'var(--font-headline)' }}>
                  {ethBalance} ETH
                </div>
                <p style={{ fontSize: '0.75rem', color: '#888', marginTop: '0.25rem', fontStyle: 'italic' }}>
                  * MetaMask Wallet Address: {userProfile?.email.split('@')[0].substring(0, 6)}...{userProfile?.email.split('@')[0].substring(38)}
                </p>
              </div>
            )}
          </div>

          {/* Deposit form */}
          <div className="border-box" style={{ background: '#FFFFFF', padding: '2rem', borderWidth: '1px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '1.5rem' }}>
              <PlusCircle size={20} />
              <span style={{ fontSize: '0.75rem', fontWeight: 700, letterSpacing: '0.1em', textTransform: 'uppercase' }}>
                Deposit via SePay (Scan VietQR)
              </span>
            </div>

            <form onSubmit={handleDeposit} style={{ display: 'flex', flexDirection: 'column', gap: '1.2rem' }}>
              <div className="form-group">
                <label className="form-label" htmlFor="deposit-amount">Deposit Amount (VND)</label>
                <div style={{ position: 'relative' }}>
                  <span style={{ position: 'absolute', left: '1rem', top: '0.85rem', fontWeight: 700, fontSize: '0.9rem' }}>₫</span>
                  <input
                    id="deposit-amount"
                    type="number"
                    min="2000"
                    step="1000"
                    className="form-input"
                    style={{ paddingLeft: '2rem', width: '100%', fontSize: '1.1rem', fontWeight: 700 }}
                    value={depositAmount}
                    onChange={(e) => setDepositAmount(e.target.value)}
                    required
                  />
                </div>
              </div>

              {/* Fast select options */}
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '0.5rem' }}>
                {['20000', '50000', '100000', '200000', '500000', '1000000'].map((val) => (
                  <button
                    key={val}
                    type="button"
                    onClick={() => setDepositAmount(val)}
                    style={{
                      padding: '0.5rem',
                      fontSize: '0.75rem',
                      fontWeight: 700,
                      background: depositAmount === val ? '#000000' : 'transparent',
                      color: depositAmount === val ? '#FFFFFF' : '#000000',
                      border: '1px solid #000000',
                      cursor: 'pointer',
                      transition: 'all 0.15s ease'
                    }}
                  >
                    {parseInt(val) >= 1000000 
                      ? `${parseInt(val)/1000000}M` 
                      : `${parseInt(val)/1000}K`
                    }
                  </button>
                ))}
              </div>

              <button
                type="submit"
                disabled={loading}
                className="btn btn-accent"
                style={{
                  width: '100%',
                  padding: '1rem',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '0.5rem',
                  marginTop: '0.5rem'
                }}
              >
                <ArrowUpRight size={18} />
                {loading ? 'INITIALIZING...' : 'CONFIRM DEPOSIT'}
              </button>
            </form>
          </div>
        </div>

        {/* Right column: Transaction history */}
        <div className="border-box" style={{ background: '#FFFFFF', padding: '2rem', borderWidth: '1px', display: 'flex', flexDirection: 'column' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '1.5rem', borderBottom: '1px solid #EEE', paddingBottom: '0.75rem' }}>
            <History size={20} />
            <span style={{ fontSize: '0.75rem', fontWeight: 700, letterSpacing: '0.1em', textTransform: 'uppercase' }}>
              Recent Transaction History
            </span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem', flex: 1 }}>
            {transactions.length === 0 ? (
              <div style={{ textAlign: 'center', margin: 'auto 0', color: '#888', padding: '2rem 0' }}>
                <CreditCard size={32} style={{ margin: '0 auto 0.75rem', opacity: 0.3 }} />
                <p style={{ fontSize: '0.75rem', letterSpacing: '0.05em', textTransform: 'uppercase' }}>No transactions recorded yet</p>
              </div>
            ) : (
              transactions.map((tx) => (
                <div 
                  key={tx.id} 
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    padding: '0.75rem',
                    border: '1px solid #EEE',
                    background: '#FAF9F6'
                  }}
                >
                  <div>
                    <div style={{ fontSize: '0.85rem', fontWeight: 700 }}>
                      Order Code: #{tx.gatewayRefId}
                    </div>
                    <div style={{ fontSize: '0.7rem', color: '#888', marginTop: '0.15rem' }}>
                      {new Date(tx.createdAt).toLocaleString('en-US')}
                    </div>
                  </div>

                  <div style={{ textAlign: 'right' }}>
                    <div style={{ fontSize: '0.9rem', fontWeight: 800, color: 'var(--accent-color)' }}>
                      +{formatVND(tx.amount)}
                    </div>
                    <div 
                      style={{ 
                        fontSize: '0.65rem', 
                        fontWeight: 700, 
                        letterSpacing: '0.05em',
                        color: tx.status === 'SUCCESS' ? '#10B981' : tx.status === 'PENDING' ? '#F59E0B' : '#EF4444',
                        textTransform: 'uppercase',
                        marginTop: '0.15rem'
                      }}
                    >
                      {tx.status}
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* SePay Bank Transfer Modal */}
      {paymentInfo && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0, 0, 0, 0.6)',
          backdropFilter: 'blur(4px)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 9999,
          padding: '1rem'
        }}>
          <div style={{
            background: '#FFFFFF',
            border: '1.5px solid #000000',
            maxWidth: '680px',
            width: '100%',
            padding: '2rem',
            position: 'relative',
            boxShadow: '0 20px 25px -5px rgba(0, 0, 0, 0.1)'
          }}>
            <button 
              onClick={() => setPaymentInfo(null)}
              style={{
                position: 'absolute',
                top: '1.5rem',
                right: '1.5rem',
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                color: '#666'
              }}
            >
              <X size={24} />
            </button>

            <h2 style={{ fontSize: '1.5rem', fontWeight: 800, letterSpacing: '-0.02em', marginBottom: '0.5rem', textTransform: 'uppercase' }}>
              VietQR Bank Transfer Details
            </h2>
            <p style={{ fontSize: '0.8rem', color: '#666', marginBottom: '1.5rem', borderBottom: '1px solid #EEE', paddingBottom: '1rem' }}>
              Please open your mobile banking application to scan the QR code below or make a manual bank transfer with the exact details.
            </p>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1.2fr', gap: '2rem' }}>
              {/* Left Column: QR Code */}
              <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', borderRight: '1px dashed #DDD', paddingRight: '1.5rem' }}>
                <div style={{ border: '1px solid #000', padding: '0.5rem', background: '#FFF' }}>
                  <img 
                    src={paymentInfo.qrUrl} 
                    alt="VietQR Transfer" 
                    style={{ width: '100%', height: 'auto', display: 'block' }}
                  />
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', marginTop: '0.75rem' }}>
                  <span className="logo-dot" style={{ width: '8px', height: '8px', background: '#10B981', display: 'inline-block', borderRadius: '50%', animation: 'pulse 1.5s infinite' }}></span>
                  <span style={{ fontSize: '0.7rem', fontWeight: 700, color: '#10B981', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    Awaiting payment confirmation...
                  </span>
                </div>
              </div>

              {/* Right Column: Bank Details */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem', justifyContent: 'center' }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem' }}>
                  <span style={{ fontSize: '0.65rem', fontWeight: 700, color: '#888', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Bank Name</span>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #EEE', paddingBottom: '0.4rem' }}>
                    <span style={{ fontSize: '0.9rem', fontWeight: 800 }}>{paymentInfo.bankName}</span>
                  </div>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem' }}>
                  <span style={{ fontSize: '0.65rem', fontWeight: 700, color: '#888', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Account Number</span>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #EEE', paddingBottom: '0.4rem' }}>
                    <span style={{ fontSize: '0.9rem', fontWeight: 800 }}>{paymentInfo.accountNo}</span>
                    <button 
                      onClick={() => copyToClipboard(paymentInfo.accountNo, 'Account Number')}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '0.2rem', color: '#666', fontSize: '0.7rem', fontWeight: 700 }}
                    >
                      {copiedField === 'Account Number' ? <Check size={12} color="#10B981" /> : <Copy size={12} />}
                      {copiedField === 'Account Number' ? 'Copied' : 'Copy'}
                    </button>
                  </div>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem' }}>
                  <span style={{ fontSize: '0.65rem', fontWeight: 700, color: '#888', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Account Owner</span>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #EEE', paddingBottom: '0.4rem' }}>
                    <span style={{ fontSize: '0.9rem', fontWeight: 800, textTransform: 'uppercase' }}>{paymentInfo.accountName}</span>
                  </div>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem' }}>
                  <span style={{ fontSize: '0.65rem', fontWeight: 700, color: '#888', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Amount</span>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #EEE', paddingBottom: '0.4rem' }}>
                    <span style={{ fontSize: '1rem', fontWeight: 900, color: 'var(--accent-color)' }}>{formatVND(paymentInfo.amount)}</span>
                  </div>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.2rem', padding: '0.75rem', background: '#FFFBEB', border: '1px solid #FDE68A' }}>
                  <span style={{ fontSize: '0.65rem', fontWeight: 700, color: '#B45309', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Transfer Memo (Required)</span>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '0.2rem' }}>
                    <span style={{ fontSize: '1.1rem', fontWeight: 900, color: '#B45309', letterSpacing: '0.05em' }}>{paymentInfo.memo}</span>
                    <button 
                      onClick={() => copyToClipboard(paymentInfo.memo, 'Transfer Memo')}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '0.2rem', color: '#B45309', fontSize: '0.7rem', fontWeight: 700 }}
                    >
                      {copiedField === 'Transfer Memo' ? <Check size={12} color="#10B981" /> : <Copy size={12} />}
                      {copiedField === 'Transfer Memo' ? 'Copied' : 'Copy'}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <div style={{ display: 'flex', gap: '1rem', marginTop: '2rem', borderTop: '1px solid #EEE', paddingTop: '1.5rem' }}>
              <button 
                onClick={() => setPaymentInfo(null)}
                style={{
                  flex: 1,
                  padding: '0.75rem',
                  fontSize: '0.75rem',
                  fontWeight: 700,
                  background: '#FFFFFF',
                  color: '#000000',
                  border: '1.5px solid #000000',
                  cursor: 'pointer',
                  textTransform: 'uppercase',
                  letterSpacing: '0.05em'
                }}
              >
                Close
              </button>
              <button 
                onClick={fetchWalletData}
                style={{
                  flex: 1,
                  padding: '0.75rem',
                  fontSize: '0.75rem',
                  fontWeight: 700,
                  background: '#000000',
                  color: '#FFFFFF',
                  border: '1.5px solid #000000',
                  cursor: 'pointer',
                  textTransform: 'uppercase',
                  letterSpacing: '0.05em'
                }}
              >
                I have transferred
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
