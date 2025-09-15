// Dashboard configuration
const API_BASE_URL = window.location.origin; // Use the same origin as the dashboard
const REFRESH_INTERVAL = 30000; // Auto-refresh every 30 seconds

console.log('Dashboard script loaded, API_BASE_URL:', API_BASE_URL);

// Global data store
let dashboardData = {
    tokens: [],
    pledgedTokens: [],
    unpledgeSequence: [],
    transactions: [],
    stats: {
        total: 0,
        pledged: 0,
        unpledged: 0,
        available: 0
    },
    currentDID: '',
    availableDIDs: []
};

// Initialize dashboard on page load
document.addEventListener('DOMContentLoaded', () => {
    console.log('Dashboard initializing...');
    initializeDashboard();
    setupEventListeners();
    loadDIDs().then(() => {
        console.log('DIDs loaded, now loading data...');
        loadData();
    }).catch(error => {
        console.error('Failed to load DIDs:', error);
        updateConnectionStatus('error');
    });
    
    // Set up auto-refresh
    setInterval(loadData, REFRESH_INTERVAL);
});

// Initialize dashboard components
function initializeDashboard() {
    // Set today's date for date filters
    const today = new Date().toISOString().split('T')[0];
    document.getElementById('toDate').value = today;
    
    // Set 30 days ago for from date
    const thirtyDaysAgo = new Date();
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);
    document.getElementById('fromDate').value = thirtyDaysAgo.toISOString().split('T')[0];
}

// Set up event listeners
function setupEventListeners() {
    // DID selector
    document.getElementById('didSelect').addEventListener('change', (e) => {
        dashboardData.currentDID = e.target.value;
        loadData();
    });
    
    // Pledged tokens filters
    document.getElementById('pledgedSearch').addEventListener('input', filterPledgedTokens);
    document.getElementById('pledgedFilter').addEventListener('change', filterPledgedTokens);
    
    // Unpledge sequence filters
    document.getElementById('unpledgeSearch').addEventListener('input', filterUnpledgeSequence);
    document.getElementById('unpledgeSort').addEventListener('change', sortUnpledgeSequence);
    
    // Transaction filters
    document.getElementById('fromDate').addEventListener('change', filterTransactions);
    document.getElementById('toDate').addEventListener('change', filterTransactions);
    document.getElementById('txType').addEventListener('change', filterTransactions);
}

// Load available DIDs
async function loadDIDs() {
    try {
        console.log('Fetching DIDs from:', `${API_BASE_URL}/api/v1/dids`);
        // Fetch all DIDs from the API
        const response = await fetch(`${API_BASE_URL}/api/v1/dids`);
        console.log('DIDs response status:', response.status, response.statusText);
        
        if (response.ok) {
            const data = await response.json();
            console.log('DIDs data received:', data);
            
            if (data.dids && data.dids.length > 0) {
                dashboardData.availableDIDs = data.dids;
                if (!dashboardData.currentDID) {
                    dashboardData.currentDID = data.dids[0];
                }
                
                // Update DID selector
                const didSelect = document.getElementById('didSelect');
                didSelect.innerHTML = dashboardData.availableDIDs.map(did => 
                    `<option value="${did}" ${did === dashboardData.currentDID ? 'selected' : ''}>${truncateString(did, 40)}</option>`
                ).join('');
                console.log('DID selector updated with', data.dids.length, 'DIDs');
            } else {
                console.log('No DIDs in response, trying dashboard data fallback...');
                // Fallback: try to get DID from dashboard data
                const dashResponse = await fetch(`${API_BASE_URL}/api/dashboard/data`);
                if (dashResponse.ok) {
                    const dashData = await dashResponse.json();
                    if (dashData.stats && dashData.stats.did) {
                        dashboardData.availableDIDs = [dashData.stats.did];
                        dashboardData.currentDID = dashData.stats.did;
                        
                        // Update DID selector
                        const didSelect = document.getElementById('didSelect');
                        didSelect.innerHTML = dashboardData.availableDIDs.map(did => 
                            `<option value="${did}" ${did === dashboardData.currentDID ? 'selected' : ''}>${truncateString(did, 40)}</option>`
                        ).join('');
                        console.log('DID selector updated from dashboard data fallback');
                    }
                }
            }
        } else {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
    } catch (error) {
        console.error('Error loading DIDs:', error);
        throw error;
    }
}

// Main data loading function
async function loadData() {
    try {
        console.log('Starting loadData...');
        updateConnectionStatus('connecting');
        
        // Build URL with DID parameter if available
        let url = `${API_BASE_URL}/api/dashboard/data`;
        if (dashboardData.currentDID) {
            url += `?did=${encodeURIComponent(dashboardData.currentDID)}`;
        }
        
        console.log('Fetching dashboard data from:', url);
        // Fetch dashboard data
        const response = await fetch(url);
        console.log('Dashboard response status:', response.status, response.statusText);
        
        if (!response.ok) throw new Error(`Failed to fetch dashboard data: ${response.status} ${response.statusText}`);
        const data = await response.json();
        
        console.log('Dashboard data received:', data);
        
        // Update global data store
        dashboardData.pledgedTokens = data.pledged_tokens || [];
        dashboardData.unpledgeSequence = data.unpledge_sequences || [];
        dashboardData.transactions = data.recent_transactions || [];
        
        // Update stats from API data
        if (data.stats) {
            dashboardData.stats = {
                total: data.stats.total_tokens || 0,
                pledged: data.stats.pledged_tokens || 0,
                unpledged: dashboardData.unpledgeSequence.length || 0,
                available: data.stats.available_tokens || 0,
                balance: data.stats.balance || 0,
                did: data.stats.did || '',
                peer_id: data.stats.peer_id || ''
            };
            
            // Update current DID if not set
            if (!dashboardData.currentDID && data.stats.did) {
                dashboardData.currentDID = data.stats.did;
                // Check if we need to update the DID list
                if (dashboardData.availableDIDs.indexOf(data.stats.did) === -1) {
                    await loadDIDs();
                }
            }
        }
        
        // Update UI
        updateStats();
        updatePledgedTokensTable();
        updateUnpledgeSequenceTable();
        updateTransactionsTable();
        
        updateConnectionStatus('connected');
        console.log('Data loading completed successfully');
    } catch (error) {
        console.error('Error loading data:', error);
        console.error('Error details:', error.message);
        updateConnectionStatus('error');
        showError('Failed to load data. Please check if RubixGo is running.');
    }
}

// Fetch tokens from API
async function fetchTokens() {
    try {
        const response = await fetch(`${API_BASE_URL}/api/v1/tokens`);
        if (!response.ok) throw new Error('Failed to fetch tokens');
        const data = await response.json();
        return data.tokens || [];
    } catch (error) {
        console.error('Error fetching tokens:', error);
        // Try alternative endpoint
        return await fetchTokensAlternative();
    }
}

// Alternative token fetching method
async function fetchTokensAlternative() {
    try {
        // Try to get account info which includes tokens
        const response = await fetch(`${API_BASE_URL}/api/v1/account-info`);
        if (!response.ok) throw new Error('Failed to fetch account info');
        const data = await response.json();
        return processAccountTokens(data);
    } catch (error) {
        console.error('Error fetching alternative tokens:', error);
        return [];
    }
}

// Process account tokens
function processAccountTokens(accountData) {
    const tokens = [];
    
    // Process different token types
    if (accountData.rbt_tokens) {
        accountData.rbt_tokens.forEach(token => {
            tokens.push({
                id: token,
                type: 0,
                value: 1,
                status: 'available'
            });
        });
    }
    
    if (accountData.part_tokens) {
        accountData.part_tokens.forEach(token => {
            tokens.push({
                id: token.token_id,
                type: 1,
                value: token.value,
                status: 'available'
            });
        });
    }
    
    return tokens;
}

// Fetch transactions from API
async function fetchTransactions() {
    try {
        const response = await fetch(`${API_BASE_URL}/api/v1/transactions`);
        if (!response.ok) throw new Error('Failed to fetch transactions');
        const data = await response.json();
        return data.transactions || [];
    } catch (error) {
        console.error('Error fetching transactions:', error);
        return []; // Return empty array instead of mock data
    }
}

// Fetch unpledge sequence data
async function fetchUnpledgeSequence() {
    try {
        const response = await fetch(`${API_BASE_URL}/api/v1/unpledge-sequence`);
        if (!response.ok) throw new Error('Failed to fetch unpledge sequence');
        const data = await response.json();
        return data.sequences || [];
    } catch (error) {
        console.error('Error fetching unpledge sequence:', error);
        return []; // Return empty array instead of mock data
    }
}

// Process pledged tokens from token list
function processPledgedTokens() {
    dashboardData.pledgedTokens = dashboardData.tokens.filter(token => 
        token.status === 'pledged' || token.pledged === true
    );
}

// Update statistics
function updateStats() {
    const stats = dashboardData.stats;
    
    // Update UI with values from API
    document.getElementById('totalTokens').textContent = formatNumber(stats.total || 0);
    document.getElementById('pledgedTokens').textContent = formatNumber(stats.pledged || 0);
    document.getElementById('unpledgedQueue').textContent = formatNumber(stats.unpledged || 0);
    document.getElementById('availableTokens').textContent = formatNumber(stats.available || 0);
    
    // Update progress bar
    const pledgedPercent = stats.total > 0 ? (stats.pledged / stats.total * 100).toFixed(1) : 0;
    const progressBar = document.getElementById('pledgedProgress');
    progressBar.style.width = `${pledgedPercent}%`;
    progressBar.textContent = `${pledgedPercent}%`;
}

// Update pledged tokens table
function updatePledgedTokensTable() {
    const tbody = document.getElementById('pledgedTableBody');
    const searchTerm = document.getElementById('pledgedSearch').value.toLowerCase();
    const filterType = document.getElementById('pledgedFilter').value;
    
    let filteredTokens = dashboardData.pledgedTokens;
    
    // Apply filters
    if (searchTerm) {
        filteredTokens = filteredTokens.filter(token => 
            (token.token_id || '').toLowerCase().includes(searchTerm) ||
            (token.did && token.did.toLowerCase().includes(searchTerm))
        );
    }
    
    if (filterType !== 'all') {
        filteredTokens = filteredTokens.filter(token => {
            const actualTokenType = determineTokenType(token);
            return actualTokenType == filterType;
        });
    }
    
    // Generate table HTML
    if (filteredTokens.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" class="loading">No pledged tokens found</td></tr>';
        return;
    }
    
    tbody.innerHTML = filteredTokens.slice(0, 100).map(token => {
        const actualTokenType = determineTokenType(token);
        const tokenValue = parseFloat(token.token_value || 1);
        
        return `
        <tr>
            <td class="token-id" title="${token.token_id}">${truncateString(token.token_id, 20)}</td>
            <td>${getTokenTypeName(actualTokenType)}</td>
            <td>${tokenValue} RBT</td>
            <td><span class="status-badge status-pledged">Pledged</span></td>
        </tr>
        `;
    }).join('');
}

// Update unpledge sequence table
function updateUnpledgeSequenceTable() {
    const tbody = document.getElementById('unpledgeTableBody');
    const searchTerm = document.getElementById('unpledgeSearch').value.toLowerCase();
    const sortBy = document.getElementById('unpledgeSort').value;
    
    let filteredSequences = dashboardData.unpledgeSequence;
    
    // Apply search filter
    if (searchTerm) {
        filteredSequences = filteredSequences.filter(seq => 
            (seq.transaction_id || '').toLowerCase().includes(searchTerm)
        );
    }
    
    // Apply sorting
    filteredSequences.sort((a, b) => {
        switch(sortBy) {
            case 'time':
                return new Date(b.initiated_at || 0) - new Date(a.initiated_at || 0);
            case 'count':
                const countA = (a.token_ids || []).length;
                const countB = (b.token_ids || []).length;
                return countB - countA;
            case 'id':
                return (a.transaction_id || '').localeCompare(b.transaction_id || '');
            default:
                return 0;
        }
    });
    
    // Generate table HTML
    if (filteredSequences.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" class="loading">No unpledge sequences found</td></tr>';
        return;
    }
    
    tbody.innerHTML = filteredSequences.slice(0, 100).map(seq => `
        <tr>
            <td class="token-id" title="${seq.transaction_id}">${truncateString(seq.transaction_id, 20)}</td>
            <td>${(seq.token_ids || []).length}</td>
            <td>${formatTimestamp(seq.initiated_at)}</td>
            <td><span class="status-badge status-${seq.status || 'pending'}">${seq.status || 'Pending'}</span></td>
        </tr>
    `).join('');
}

// Update transactions table
function updateTransactionsTable() {
    const tbody = document.getElementById('transactionTableBody');
    const fromDate = document.getElementById('fromDate').value;
    const toDate = document.getElementById('toDate').value;
    const txType = document.getElementById('txType').value;
    
    let filteredTransactions = dashboardData.transactions;
    
    // Apply date filter
    if (fromDate) {
        filteredTransactions = filteredTransactions.filter(tx => 
            new Date(tx.date_time || tx.timestamp) >= new Date(fromDate)
        );
    }
    
    if (toDate) {
        filteredTransactions = filteredTransactions.filter(tx => 
            new Date(tx.date_time || tx.timestamp) <= new Date(toDate + 'T23:59:59')
        );
    }
    
    // Apply type filter
    if (txType !== 'all') {
        filteredTransactions = filteredTransactions.filter(tx => 
            (tx.transaction_type || tx.type) === txType
        );
    }
    
    // Generate table HTML
    if (filteredTransactions.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="loading">No transactions found</td></tr>';
        return;
    }
    
    tbody.innerHTML = filteredTransactions.slice(0, 100).map(tx => `
        <tr>
            <td class="token-id" title="${tx.transaction_id || tx.id}">${truncateString(tx.transaction_id || tx.id, 15)}</td>
            <td>${tx.transaction_type || tx.type || 'transfer'}</td>
            <td>${tx.amount || 0} RBT</td>
            <td title="${tx.sender_did || tx.from || '-'}">${truncateString(tx.sender_did || tx.from || '-', 10)}</td>
            <td title="${tx.receiver_did || tx.to || '-'}">${truncateString(tx.receiver_did || tx.to || '-', 10)}</td>
            <td>${formatTimestamp(tx.date_time || tx.timestamp)}</td>
            <td><span class="status-badge status-${getStatusClass(tx.status)}">${tx.status || 'completed'}</span></td>
        </tr>
    `).join('');
}

// Filter functions
function filterPledgedTokens() {
    updatePledgedTokensTable();
}

function filterUnpledgeSequence() {
    updateUnpledgeSequenceTable();
}

function sortUnpledgeSequence() {
    updateUnpledgeSequenceTable();
}

function filterTransactions() {
    updateTransactionsTable();
}

// Refresh data manually
function refreshData() {
    const btn = document.querySelector('.refresh-btn');
    btn.style.transform = 'rotate(360deg)';
    loadData().then(() => {
        setTimeout(() => {
            btn.style.transform = 'rotate(0deg)';
        }, 500);
    });
}

// Update connection status indicator
function updateConnectionStatus(status) {
    const statusEl = document.getElementById('connectionStatus');
    
    switch(status) {
        case 'connected':
            statusEl.textContent = '✅ Connected';
            statusEl.className = 'connection-status status-connected';
            break;
        case 'connecting':
            statusEl.textContent = '⏳ Connecting...';
            statusEl.className = 'connection-status status-disconnected';
            break;
        case 'error':
            statusEl.textContent = '❌ Disconnected';
            statusEl.className = 'connection-status status-disconnected';
            break;
    }
}

// Show error message
function showError(message) {
    // You can enhance this to show a modal or toast notification
    console.error(message);
}

// Utility functions
function formatNumber(num) {
    return new Intl.NumberFormat().format(num);
}

function truncateString(str, maxLength) {
    if (!str) return '-';
    if (str.length <= maxLength) return str;
    return str.substring(0, maxLength) + '...';
}

function formatTimestamp(timestamp) {
    if (!timestamp) return '-';
    const date = new Date(timestamp);
    return date.toLocaleString();
}

function getTokenTypeName(type) {
    const types = {
        0: 'RBT',
        1: 'Part',
        4: 'FT'
    };
    return types[type] || 'Unknown';
}

// Determine token type based on value and type
function determineTokenType(token) {
    // FT tokens have explicit type 4
    if (token.token_type === 4) {
        return 4; // FT
    }
    
    // For other tokens, determine by value
    const value = parseFloat(token.token_value || 1);
    if (value === 1) {
        return 0; // RBT (whole token)
    } else {
        return 1; // Part token (decimal value)
    }
}

function getStatusClass(status) {
    if (!status) return 'pending';
    status = status.toLowerCase();
    if (status.includes('success') || status.includes('complete')) return 'pledged';
    if (status.includes('fail') || status.includes('error')) return 'unpledged';
    return 'pending';
}

// Generate mock data for demonstration
function generateMockTransactions() {
    const mockTransactions = [];
    const types = ['pledge', 'unpledge', 'transfer'];
    const statuses = ['completed', 'pending', 'failed'];
    
    for (let i = 0; i < 20; i++) {
        const date = new Date();
        date.setDate(date.getDate() - Math.floor(Math.random() * 30));
        
        mockTransactions.push({
            id: 'tx_' + Math.random().toString(36).substr(2, 9),
            type: types[Math.floor(Math.random() * types.length)],
            amount: Math.floor(Math.random() * 100) + 1,
            from: 'bafybmi' + Math.random().toString(36).substr(2, 10),
            to: 'bafybmi' + Math.random().toString(36).substr(2, 10),
            timestamp: date.toISOString(),
            status: statuses[Math.floor(Math.random() * statuses.length)]
        });
    }
    
    return mockTransactions;
}

function generateMockUnpledgeData() {
    const mockData = [];
    
    for (let i = 0; i < 10; i++) {
        const date = new Date();
        date.setHours(date.getHours() - Math.floor(Math.random() * 24));
        
        mockData.push({
            transaction_id: 'unpledge_' + Math.random().toString(36).substr(2, 9),
            token_count: Math.floor(Math.random() * 10) + 1,
            timestamp: date.toISOString(),
            status: 'pending'
        });
    }
    
    return mockData;
}