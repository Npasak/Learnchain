// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title SoulboundLearningLedger
 * @dev Educational On-Chain Ledger & Non-Transferable ERC-721 Soulbound Token (SBT).
 * Records on-chain:
 *  - Lesson Attendance & Watch Time
 *  - Quiz Assessment Scores & Completion
 *  - Verifiable Non-Transferable Soulbound Certificate
 */

interface IERC165 {
    function supportsInterface(bytes4 interfaceId) external view returns (bool);
}

interface IERC721 is IERC165 {
    event Transfer(address indexed from, address indexed to, uint256 indexed tokenId);
    event Approval(address indexed owner, address indexed approved, uint256 indexed tokenId);
    event ApprovalForAll(address indexed owner, address indexed operator, bool approved);

    function balanceOf(address owner) external view returns (uint256 balance);
    function ownerOf(uint256 tokenId) external view returns (address owner);
    function safeTransferFrom(address from, address to, uint256 tokenId, bytes calldata data) external;
    function safeTransferFrom(address from, address to, uint256 tokenId) external;
    function transferFrom(address from, address to, uint256 tokenId) external;
    function approve(address to, uint256 tokenId) external;
    function setApprovalForAll(address operator, bool approved) external;
    function getApproved(uint256 tokenId) external view returns (address operator);
    function isApprovedForAll(address owner, address operator) external view returns (bool);
}

interface IERC721Metadata is IERC721 {
    function name() external view returns (string memory);
    function symbol() external view returns (string memory);
    function tokenURI(uint256 tokenId) external view returns (string memory);
}

contract SoulboundCertificate is IERC721Metadata {
    string private _name;
    string private _symbol;
    address public owner;
    uint256 private _nextTokenId;

    // --- On-Chain Learning Statistics & Structures ---
    struct AttendanceRecord {
        uint256 courseId;
        uint256 lessonId;
        uint256 watchSeconds;
        uint256 timestamp;
    }

    struct QuizRecord {
        uint256 quizId;
        uint256 score;
        uint256 totalScore;
        bool isPassed;
        uint256 timestamp;
    }

    struct CertificateStats {
        uint256 courseId;
        uint256 totalLessonsCompleted;
        uint256 totalWatchSeconds;
        uint256 averageScorePercent;
        uint256 issuedAt;
    }

    // Learner => LessonID => Attendance
    mapping(address => mapping(uint256 => AttendanceRecord)) public attendanceRecords;
    // Learner => QuizID => QuizRecord
    mapping(address => mapping(uint256 => QuizRecord)) public quizRecords;
    // TokenID => CertificateStats
    mapping(uint256 => CertificateStats) public certificateStats;
    // User Address + Course ID => Has Claimed Certificate
    mapping(bytes32 => bool) public hasClaimed;

    // ERC-721 Internal State
    mapping(uint256 => address) private _owners;
    mapping(address => uint256) private _balances;
    mapping(uint256 => string) private _tokenURIs;

    // --- Events ---
    event AttendanceRecorded(address indexed learner, uint256 indexed courseId, uint256 indexed lessonId, uint256 watchSeconds, uint256 timestamp);
    event QuizRecorded(address indexed learner, uint256 indexed quizId, uint256 score, uint256 totalScore, bool isPassed, uint256 timestamp);
    event CertificateMinted(address indexed recipient, uint256 indexed tokenId, uint256 indexed courseId, uint256 averageScore, uint256 totalWatchSeconds, string uri);

    modifier onlyOwner() {
        require(msg.sender == owner, "Only contract owner or relayer can execute");
        _;
    }

    constructor(string memory name_, string memory symbol_) {
        _name = name_;
        _symbol = symbol_;
        owner = msg.sender;
        _nextTokenId = 1;
    }

    // =========================================================================
    // 1. ON-CHAIN LEARNING ATTENDANCE & QUIZ SCORES
    // =========================================================================

    /**
     * @notice Records a learner's lesson attendance and study time directly on blockchain
     */
    function recordAttendance(
        address learner,
        uint256 courseId,
        uint256 lessonId,
        uint256 watchSeconds
    ) external onlyOwner {
        require(learner != address(0), "Invalid learner address");
        attendanceRecords[learner][lessonId] = AttendanceRecord({
            courseId: courseId,
            lessonId: lessonId,
            watchSeconds: watchSeconds,
            timestamp: block.timestamp
        });

        emit AttendanceRecorded(learner, courseId, lessonId, watchSeconds, block.timestamp);
    }

    /**
     * @notice Records quiz evaluation results and test scores on blockchain
     */
    function recordQuizResult(
        address learner,
        uint256 quizId,
        uint256 score,
        uint256 totalScore,
        bool isPassed
    ) external onlyOwner {
        require(learner != address(0), "Invalid learner address");
        quizRecords[learner][quizId] = QuizRecord({
            quizId: quizId,
            score: score,
            totalScore: totalScore,
            isPassed: isPassed,
            timestamp: block.timestamp
        });

        emit QuizRecorded(learner, quizId, score, totalScore, isPassed, block.timestamp);
    }

    // =========================================================================
    // 2. SOULBOUND CERTIFICATE MINTING
    // =========================================================================

    /**
     * @notice Mint a Soulbound Certificate with comprehensive learning statistics
     */
    function mintCertificate(
        address to,
        uint256 courseId,
        uint256 lessonsCompleted,
        uint256 totalWatchSeconds,
        uint256 avgScorePercent,
        string memory uri
    ) external onlyOwner returns (uint256) {
        require(to != address(0), "Cannot mint to zero address");
        bytes32 claimKey = keccak256(abi.encodePacked(to, courseId));
        require(!hasClaimed[claimKey], "Certificate already issued for this course");

        uint256 tokenId = _nextTokenId++;
        _owners[tokenId] = to;
        _balances[to] += 1;
        _tokenURIs[tokenId] = uri;
        hasClaimed[claimKey] = true;

        certificateStats[tokenId] = CertificateStats({
            courseId: courseId,
            totalLessonsCompleted: lessonsCompleted,
            totalWatchSeconds: totalWatchSeconds,
            averageScorePercent: avgScorePercent,
            issuedAt: block.timestamp
        });

        emit Transfer(address(0), to, tokenId);
        emit CertificateMinted(to, tokenId, courseId, avgScorePercent, totalWatchSeconds, uri);

        return tokenId;
    }

    // =========================================================================
    // 3. ERC-721 METADATA & GETTERS
    // =========================================================================

    function name() public view override returns (string memory) {
        return _name;
    }

    function symbol() public view override returns (string memory) {
        return _symbol;
    }

    function tokenURI(uint256 tokenId) public view override returns (string memory) {
        require(_owners[tokenId] != address(0), "Token does not exist");
        return _tokenURIs[tokenId];
    }

    function balanceOf(address account) public view override returns (uint256) {
        require(account != address(0), "Address zero is invalid");
        return _balances[account];
    }

    function ownerOf(uint256 tokenId) public view override returns (address) {
        address tokenOwner = _owners[tokenId];
        require(tokenOwner != address(0), "Token does not exist");
        return tokenOwner;
    }

    // =========================================================================
    // 4. SOULBOUND RESTRICTIONS: ALL TRANSFERS & APPROVALS REVERT
    // =========================================================================

    function transferFrom(address, address, uint256) public pure override {
        revert("Soulbound: Token cannot be transferred");
    }

    function safeTransferFrom(address, address, uint256) public pure override {
        revert("Soulbound: Token cannot be transferred");
    }

    function safeTransferFrom(address, address, uint256, bytes calldata) public pure override {
        revert("Soulbound: Token cannot be transferred");
    }

    function approve(address, uint256) public pure override {
        revert("Soulbound: Token cannot be approved for trading");
    }

    function setApprovalForAll(address, bool) public pure override {
        revert("Soulbound: Operator approval forbidden for SBT");
    }

    function getApproved(uint256) public pure override returns (address) {
        return address(0);
    }

    function isApprovedForAll(address, address) public pure override returns (bool) {
        return false;
    }

    function supportsInterface(bytes4 interfaceId) public pure override returns (bool) {
        return interfaceId == type(IERC721).interfaceId ||
               interfaceId == type(IERC721Metadata).interfaceId ||
               interfaceId == type(IERC165).interfaceId;
    }
}
