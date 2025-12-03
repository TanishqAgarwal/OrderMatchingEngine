wrk.method = "POST"
wrk.headers["Content-Type"] = "application/json"

-- Pre-define 100 symbols to rotate through for maximum concurrency
local symbol_strings = {
    "BTC-USD", "ETH-USD", "SOL-USD", "XRP-USD", "ADA-USD",
    "DOGE-USD", "DOT-USD", "LTC-USD", "SHIB-USD", "TRX-USD",
    "AVAX-USD", "MATIC-USD", "WBTC-USD", "UNI-USD", "LINK-USD",
    "ATOM-USD", "LEO-USD", "XMR-USD", "ETC-USD", "BCH-USD",
    "ALGO-USD", "APE-USD", "FIL-USD", "HBAR-USD", "ICP-USD",
    "MANA-USD", "SAND-USD", "VET-USD", "AXS-USD", "EGLD-USD",
    "EOS-USD", "FLOW-USD", "FTM-USD", "GRT-USD", "KLAY-USD",
    "LUNC-USD", "MKR-USD", "NEAR-USD", "QNT-USD", "RUNE-USD",
    "THETA-USD", "XTZ-USD", "ZEC-USD", "AAVE-USD", "BSV-USD",
    "CRV-USD", "DASH-USD", "ENJ-USD", "GALA-USD", "KSM-USD",
    "MINA-USD", "STX-USD", "CAKE-USD", "CHZ-USD", "CVX-USD",
    "DECR-USD", "DYDX-USD", "ELON-USD", "ENS-USD", "FEI-USD",
    "FXS-USD", "GNO-USD", "HOT-USD", "HT-USD", "IMX-USD",
    "IOTX-USD", "JST-USD", "KAVA-USD", "KDA-USD", "LRC-USD",
    "LUNA-USD", "NEXO-USD", "OKB-USD", "ONE-USD", "PAXG-USD",
    "COMP-USD", "QTUM-USD", "ROSE-USD", "RVN-USD", "SNX-USD",
    "SUSHI-USD", "TFUEL-USD", "TUSD-USD", "USDP-USD", "WAVES-USD",
    "XEM-USD", "XEC-USD", "YFI-USD", "ZIL-USD", "CELO-USD",
    "AMP-USD", "AR-USD", "BAT-USD", "BNT-USD", "BTG-USD",
    "APPL-USD", "NVIDIA-USD", "MSFT-USD", "XCOM-USD", "GOOG-USD",
}

-- Pre-generate request bodies to save CPU during test
local request_bodies = {}
for i, symbol in ipairs(symbol_strings) do
    request_bodies[i] = '{"symbol": "' .. symbol .. '", "side": "BUY", "type": "LIMIT", "price": 5000000, "quantity": 10}'
end

-- Counter to rotate through symbols
local counter = 0
local num_symbols = #symbol_strings

function request()
    -- Rotate symbol index
    local index = (counter % num_symbols) + 1
    counter = counter + 1
    
    -- Use pre-generated body
    wrk.body = request_bodies[index]
    
    return wrk.format(nil, nil)
end
