// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title NFTAuctionMarket
 * @dev Hợp đồng thông minh mô phỏng chuẩn ERC-721 đơn giản cho đấu giá và chuyển giao NFT.
 * Giúp định nghĩa rõ vai trò Blockchain (Source of Truth) cho quyền sở hữu tài sản.
 */
contract NFTAuctionMarket {
    // Tên và ký hiệu của NFT Collection
    string public name = "Curatorial NFT Collection";
    string public symbol = "CURATOR";

    // Cấu trúc dữ liệu đại diện cho NFT
    struct NFT {
        uint256 id;
        string tokenURI;
        address creator;
        address owner;
        uint256 currentPrice;
        bool isSold;
    }

    // Lưu vết Token ID tiếp theo được đúc
    uint256 private _nextTokenId;

    // Ánh xạ từ TokenID sang thông tin NFT chi tiết
    mapping(uint256 => NFT) private _nfts;
    // Ánh xạ sở hữu: TokenID => Địa chỉ chủ sở hữu
    mapping(uint256 => address) private _tokenOwners;
    // Ánh xạ số lượng NFT sở hữu của một địa chỉ: Địa chỉ => Số lượng
    mapping(address => uint256) private _balances;

    // Các sự kiện On-chain phục vụ Go Event Listener lắng nghe và đồng bộ database
    event NFTMinted(uint256 indexed tokenId, string tokenURI, address indexed creator);
    event NFTPurchased(uint256 indexed tokenId, address indexed previousOwner, address indexed newOwner, uint256 price);
    event Transfer(address indexed from, address indexed to, uint256 indexed tokenId);

    // Modifier kiểm tra quyền sở hữu NFT
    modifier onlyOwnerOf(uint256 tokenId) {
        require(_tokenOwners[tokenId] == msg.sender, "NFTAuctionMarket: Ban khong phai chu so huu NFT nay");
        _;
    }

    /**
     * @dev Đúc (Mint) một tác phẩm NFT mới lên Blockchain
     * @param tokenURI Hash IPFS trỏ tới tệp JSON chứa metadata nghệ thuật
     */
    function mintNFT(string memory tokenURI) public returns (uint256) {
        _nextTokenId++;
        uint256 newTokenId = _nextTokenId;

        _nfts[newTokenId] = NFT({
            id: newTokenId,
            tokenURI: tokenURI,
            creator: msg.sender,
            owner: msg.sender,
            currentPrice: 0,
            isSold: false
        });

        _tokenOwners[newTokenId] = msg.sender;
        _balances[msg.sender]++;

        emit NFTMinted(newTokenId, tokenURI, msg.sender);
        emit Transfer(address(0), msg.sender, newTokenId);

        return newTokenId;
    }

    /**
     * @dev Quyết toán giao dịch thầu và chuyển giao quyền sở hữu NFT (Settlement)
     * @param tokenId Mã số NFT đấu giá
     * @param newOwner Địa chỉ ví của người thắng thầu
     * @param price Giá thầu quyết định
     */
    function safeTransferFrom(uint256 tokenId, address newOwner, uint256 price) public {
        address previousOwner = _tokenOwners[tokenId];
        require(previousOwner != address(0), "NFTAuctionMarket: NFT khong ton tai");
        require(newOwner != address(0), "NFTAuctionMarket: Dia chi nguoi nhan khong hop le");

        // Cập nhật thông tin NFT
        _nfts[tokenId].owner = newOwner;
        _nfts[tokenId].currentPrice = price;
        _nfts[tokenId].isSold = true;

        // Cập nhật số dư và ánh xạ sở hữu
        _balances[previousOwner]--;
        _balances[newOwner]++;
        _tokenOwners[tokenId] = newOwner;

        emit NFTPurchased(tokenId, previousOwner, newOwner, price);
        emit Transfer(previousOwner, newOwner, tokenId);
    }

    // Các hàm Helper truy vấn trạng thái On-chain
    function ownerOf(uint256 tokenId) public view returns (address) {
        address owner = _tokenOwners[tokenId];
        require(owner != address(0), "NFTAuctionMarket: NFT khong ton tai");
        return owner;
    }

    // Lấy số lượng NFT
    function balanceOf(address owner) public view returns (uint256) {
        require(owner != address(0), "NFTAuctionMarket: Dia chi khong hop le");
        return _balances[owner];
    }

    // Lấy thông tin NFT chi tiết
    function getNFTDetails(uint256 tokenId) public view returns (
        uint256 id,
        string memory tokenURI,
        address creator,
        address owner,
        uint256 currentPrice,
        bool isSold
    ) {
        NFT memory nft = _nfts[tokenId];
        require(nft.id != 0, "NFTAuctionMarket: NFT khong ton tai");
        return (nft.id, nft.tokenURI, nft.creator, nft.owner, nft.currentPrice, nft.isSold);
    }
}
