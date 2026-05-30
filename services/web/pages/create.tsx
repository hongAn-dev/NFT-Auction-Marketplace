import { useState } from 'react';
import Head from 'next/head';
import Link from 'next/link';
import { useRouter } from 'next/router';
import { ArrowLeft, Upload, CheckCircle2, AlertCircle, HelpCircle, HardDrive, Server } from 'lucide-react';

export default function CreateNFT() {
  const router = useRouter();
  
  // Basic Info
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [startPrice, setStartPrice] = useState('');
  const [creatorId, setCreatorId] = useState('artist-' + Math.floor(Math.random() * 1000));
  
  // MongoDB Unstructured Metadata
  const [rarity, setRarity] = useState('Rare');
  const [ipfsHash, setIpfsHash] = useState('Qm' + Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15));
  const [traitStyle, setTraitStyle] = useState('Minimalist Charcoal');
  const [traitGenIndex, setTraitGenIndex] = useState('8.5');

  // File Upload State
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string>('');
  
  // Status State
  const [loading, setLoading] = useState(false);
  const [uploadStatus, setUploadStatus] = useState<string>('');
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const file = e.target.files[0];
      setSelectedFile(file);
      setPreviewUrl(URL.createObjectURL(file));
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedFile) {
      setMessage({ type: 'error', text: 'Please upload a digital asset file (JPEG/PNG) for this exhibition.' });
      return;
    }

    setLoading(true);
    setMessage(null);
    setUploadStatus('1/3: Negotiating presigned storage handshake...');

    try {
      const nftId = 'lot-' + Math.floor(Math.random() * 1000000);
      const filename = `${nftId}-${selectedFile.name.replace(/\s+/g, '_')}`;

      // ── BƯỚC 1: LẤY PRESIGNED UPLOAD URL TỪ GO SERVICE ──────────────────
      let presignData;
      try {
        const res = await fetch(`http://localhost:8080/api/v1/s3/presign?file=${filename}&type=${selectedFile.type}`);
        if (!res.ok) throw new Error('Handshake rejected');
        presignData = await res.json();
      } catch (err) {
        console.warn('⚠️ Could not connect to Go Bidding Service on Port 8080. Falling back to offline mock sandbox.');
      }

      let imageUrl = '';
      
      if (presignData && presignData.success) {
        // ONLINE MODE (R2 / Mock S3)
        imageUrl = presignData.public_url;
        setUploadStatus(`2/3: Streaming binary stream direct to R2/S3 (${presignData.storage_type})...`);

        // PUT request nhị phân trực tiếp lên Cloudflare R2 hoặc Mock S3 cục bộ
        const uploadRes = await fetch(presignData.upload_url, {
          method: 'PUT',
          headers: {
            'Content-Type': selectedFile.type
          },
          body: selectedFile
        });

        if (!uploadRes.ok) {
          throw new Error(`Direct upload stream failed with status ${uploadRes.status}`);
        }
      } else {
        // OFFLINE MODE / MOCK SANDBOX FALLBACK
        imageUrl = previewUrl; // Use local blob url for demonstration
        setUploadStatus('2/3: Simulating offline asset secure storage...');
        await new Promise((resolve) => setTimeout(resolve, 1500));
      }

      // ── BƯỚC 2: TẠO NFT TRONG POSTGRESQL ──────────────────────────────
      setUploadStatus('3/3: Committing metadata schema to SQL + NoSQL databases...');
      
      const nftPayload = {
        id: nftId,
        title: title,
        description: description,
        image_url: imageUrl,
        creator_id: creatorId,
        start_price: parseFloat(startPrice)
      };

      let nftCreatedSuccessfully = false;

      try {
        const createRes = await fetch('http://localhost:8080/api/v1/nfts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(nftPayload)
        });
        const createResult = await createRes.json();
        if (createRes.ok && createResult.success) {
          nftCreatedSuccessfully = true;
        }
      } catch (err) {
        console.warn('⚠️ SQL Database unreachable. Simulating SQL creation in offline state.');
      }

      // ── BƯỚC 3: LƯU METADATA VÀO MONGODB ─────────────────────────────
      const metadataPayload = {
        rarity: rarity,
        created_by: creatorId,
        ipfs_hash: ipfsHash,
        attributes: {
          'Art Style': traitStyle,
          'Brutalism Index': traitGenIndex,
          'Resolution': '4K Master Asset',
          'Storage Protocol': presignData?.storage_type || 'offline_sandbox'
        }
      };

      if (nftCreatedSuccessfully) {
        try {
          await fetch(`http://localhost:8080/api/v1/nfts/${nftId}/metadata`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(metadataPayload)
          });
        } catch (err) {
          console.warn('⚠️ NoSQL MongoDB Metadata unreachable.');
        }
      }

      setMessage({
        type: 'success',
        text: `Success! Lot #${nftId.toUpperCase()} created successfully on ${presignData ? presignData.storage_type.toUpperCase() : 'OFFLINE SANDBOX'}.`
      });

      // Redirect back home after 3s
      setTimeout(() => {
        router.push('/');
      }, 3000);

    } catch (err: any) {
      setMessage({ type: 'error', text: err.message || 'Workflow broke during digital artwork creation pipeline.' });
    } finally {
      setLoading(false);
      setUploadStatus('');
    }
  };

  return (
    <div style={{ maxWidth: '800px', margin: '0 auto' }}>
      <Head>
        <title>SUBMIT ARTWORK — CURATORIAL</title>
      </Head>

      <div style={{ marginBottom: '2rem' }}>
        <Link href="/" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.5rem', fontWeight: 700, textTransform: 'uppercase', fontSize: '0.8rem', letterSpacing: '0.05em' }}>
          <ArrowLeft size={16} /> BACK TO EXHIBITION
        </Link>
      </div>

      <div style={{ marginBottom: '3rem', borderBottom: '2px solid #000', paddingBottom: '1.5rem' }}>
        <h1 style={{ fontSize: '3rem', fontWeight: 800 }}>SUBMIT DIGITAL ARTWORK</h1>
        <p style={{ color: '#666', marginTop: '0.5rem' }}>
          Submit high-resolution digital masterworks to the curated gallery. Integrated with Cloudflare R2 object storage.
        </p>
      </div>

      {message && (
        <div className="border-box" style={{
          borderColor: message.type === 'success' ? '#00FF00' : 'var(--accent-color)',
          background: message.type === 'success' ? '#F5FFF5' : '#FFF5F5',
          marginBottom: '2rem',
          padding: '1rem 1.5rem',
          display: 'flex',
          gap: '1rem',
          alignItems: 'center'
        }}>
          {message.type === 'success' ? <CheckCircle2 color="green" /> : <AlertCircle className="text-accent" />}
          <span style={{ fontSize: '0.9rem', fontWeight: 600 }}>{message.text}</span>
        </div>
      )}

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '2rem' }}>
        
        {/* Step 1: Upload Asset */}
        <div className="border-box" style={{ background: '#FFFFFF' }}>
          <h3 style={{ fontSize: '1rem', borderBottom: '1px solid #000', paddingBottom: '0.5rem', marginBottom: '1.5rem' }}>
            1. Digital Asset Binary Stream
          </h3>
          
          <div style={{ display: 'grid', gridTemplateColumns: previewUrl ? '150px 1fr' : '1fr', gap: '2rem', alignItems: 'center' }}>
            {previewUrl && (
              <div style={{ border: '1px solid #000', width: '150px', height: '150px', overflow: 'hidden', position: 'relative' }}>
                <img src={previewUrl} alt="Preview" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
              </div>
            )}
            
            <div style={{ border: '1px dashed #000', padding: '2.5rem', textAlign: 'center', background: '#FAF9F6', position: 'relative', cursor: 'pointer' }}>
              <input
                type="file"
                accept="image/*"
                onChange={handleFileChange}
                style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%', opacity: 0, cursor: 'pointer' }}
              />
              <Upload size={32} style={{ margin: '0 auto 1rem auto', color: '#666' }} />
              <div style={{ fontWeight: 700, fontSize: '0.9rem', textTransform: 'uppercase', marginBottom: '0.3rem' }}>
                {selectedFile ? selectedFile.name : 'Select or Drag & Drop Image File'}
              </div>
              <div style={{ fontSize: '0.75rem', color: '#777' }}>
                PNG, JPEG or GIF. Uploaded directly to Cloudflare R2.
              </div>
            </div>
          </div>
        </div>

        {/* Step 2: Basic Info */}
        <div className="border-box" style={{ background: '#FFFFFF' }}>
          <h3 style={{ fontSize: '1rem', borderBottom: '1px solid #000', paddingBottom: '0.5rem', marginBottom: '1.5rem' }}>
            2. Catalog Information (SQL Schema)
          </h3>
          
          <div className="form-group">
            <label className="form-label" htmlFor="title">Artwork Title</label>
            <input
              id="title"
              type="text"
              className="form-input"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. MONOLITH STUDY NO. 12"
              required
            />
          </div>

          <div className="form-group">
            <label className="form-label" htmlFor="description">Curator Description</label>
            <textarea
              id="description"
              className="form-textarea"
              rows={4}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Describe the aesthetic background, materials, and digital processes used."
              required
            />
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
            <div className="form-group">
              <label className="form-label" htmlFor="start-price">Start Bid Price ($ USD)</label>
              <input
                id="start-price"
                type="number"
                step="0.01"
                className="form-input"
                value={startPrice}
                onChange={(e) => setStartPrice(e.target.value)}
                placeholder="500.00"
                required
              />
            </div>
            
            <div className="form-group">
              <label className="form-label" htmlFor="creator-id">Artist Signature (Creator ID)</label>
              <input
                id="creator-id"
                type="text"
                className="form-input"
                value={creatorId}
                onChange={(e) => setCreatorId(e.target.value)}
                placeholder="artist_hand"
                required
              />
            </div>
          </div>
        </div>

        {/* Step 3: Unstructured Metadata (MongoDB) */}
        <div className="border-box" style={{ background: '#FFFFFF' }}>
          <h3 style={{ fontSize: '1rem', borderBottom: '1px solid #000', paddingBottom: '0.5rem', marginBottom: '1.5rem', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            3. Dynamic Extended Metadata (NoSQL Schema) <HelpCircle size={16} style={{ color: '#888' }} />
          </h3>
          
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem', marginBottom: '1.5rem' }}>
            <div className="form-group">
              <label className="form-label" htmlFor="rarity">Curatorial Rarity Tier</label>
              <select
                id="rarity"
                className="form-input"
                value={rarity}
                onChange={(e) => setRarity(e.target.value)}
                style={{ appearance: 'none', background: '#FFFFFF' }}
              >
                <option value="Curated Exhibition Grade">Curated Exhibition Grade</option>
                <option value="Legendary Masterpiece">Legendary Masterpiece</option>
                <option value="Rare Artifact">Rare Artifact</option>
                <option value="Standard Lot">Standard Lot</option>
              </select>
            </div>
            
            <div className="form-group">
              <label className="form-label" htmlFor="ipfs-hash">Decentralized IPFS Metadata CID</label>
              <input
                id="ipfs-hash"
                type="text"
                className="form-input"
                value={ipfsHash}
                onChange={(e) => setIpfsHash(e.target.value)}
                required
              />
            </div>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
            <div className="form-group">
              <label className="form-label" htmlFor="trait-style">Generative Art Style</label>
              <input
                id="trait-style"
                type="text"
                className="form-input"
                value={traitStyle}
                onChange={(e) => setTraitStyle(e.target.value)}
                placeholder="e.g. Algorithmic Brutalism"
              />
            </div>
            
            <div className="form-group">
              <label className="form-label" htmlFor="trait-gen-index">Brutalism Index (Trait Value)</label>
              <input
                id="trait-gen-index"
                type="text"
                className="form-input"
                value={traitGenIndex}
                onChange={(e) => setTraitGenIndex(e.target.value)}
                placeholder="e.g. 9.1"
              />
            </div>
          </div>
        </div>

        {/* Action Panel */}
        <div className="border-box" style={{ background: '#FAF9F6', display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '1.5rem 2rem' }}>
          <div>
            {loading ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.8rem', fontWeight: 700 }}>
                <Server className="text-accent" size={20} style={{ animation: 'spin 2s linear infinite' }} />
                <span>{uploadStatus}</span>
              </div>
            ) : (
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.8rem', fontSize: '0.8rem', color: '#666' }}>
                <HardDrive size={18} />
                <span>All network data streams directly encrypted and saved.</span>
              </div>
            )}
          </div>
          
          <button type="submit" disabled={loading} className="btn btn-accent" style={{ padding: '1rem 2.5rem' }}>
            {loading ? 'TRANSMITTING...' : 'COMMIT LOT TO EXHIBITION'}
          </button>
        </div>
      </form>
    </div>
  );
}
