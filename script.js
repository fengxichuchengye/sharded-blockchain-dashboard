const statSize = document.querySelector("#stat-size");
const statTx = document.querySelector("#stat-tx");
const statGenesis = document.querySelector("#stat-genesis");
const statUsers = document.querySelector("#stat-users");
const shardList = document.querySelector("#shard-list");
const blockTemplate = document.querySelector("#block-template");

function generateHash() {
  return Array.from({ length: 64 }, () =>
    "0123456789abcdef"[(Math.random() * 16) | 0]
  ).join("");
}

function getMockData() {
  const now = new Date();
  const blocks0 = [];
  for (let i = 0; i < 30; i++) {
    const ts = new Date(now - (29 - i) * 12 * 1000);
    const hash = generateHash();
    blocks0.push({
      blockId: i + 1,
      height: 944 - 29 + i,
      txCount: (i % 7) + 2,
      hash,
      hashLast8: hash.slice(-8),
      parentHash: generateHash(),
      timestamp: ts.toLocaleString("zh-CN", { hour12: false }),
      timestampRaw: ts.getTime() * 1e6,
      isLatest: i === 29,
    });
  }
  const blocks1 = [];
  for (let i = 0; i < 30; i++) {
    const ts = new Date(now - (29 - i) * 10 * 1000);
    const hash = generateHash();
    blocks1.push({
      blockId: i + 1,
      height: 1207 - 29 + i,
      txCount: (i % 5) + 3,
      hash,
      hashLast8: hash.slice(-8),
      parentHash: generateHash(),
      timestamp: ts.toLocaleString("zh-CN", { hour12: false }),
      timestampRaw: ts.getTime() * 1e6,
      isLatest: i === 29,
    });
  }
  const blocks2 = [];
  for (let i = 0; i < 30; i++) {
    const ts = new Date(now - (29 - i) * 15 * 1000);
    const hash = generateHash();
    blocks2.push({
      blockId: i + 1,
      height: 653 - 29 + i,
      txCount: (i % 9) + 1,
      hash,
      hashLast8: hash.slice(-8),
      parentHash: generateHash(),
      timestamp: ts.toLocaleString("zh-CN", { hour12: false }),
      timestampRaw: ts.getTime() * 1e6,
      isLatest: i === 29,
    });
  }

  const shards = [
    { id: 0, label: "Shard 0", blockCount: 30, latestHeight: 944, latestHash: blocks0[29].hash, blocks: blocks0 },
    { id: 1, label: "Shard 1", blockCount: 30, latestHeight: 1207, latestHash: blocks1[29].hash, blocks: blocks1 },
    { id: 2, label: "Shard 2", blockCount: 30, latestHeight: 653, latestHash: blocks2[29].hash, blocks: blocks2 },
  ];

  const totalTx = shards.reduce((sum, s) => sum + s.blocks.reduce((b, blk) => b + blk.txCount, 0), 0);

  return {
    stat: {
      blockchainSizeText: "1.84 GB",
      totalTransactions: totalTx,
      genesisBlockTime: "2024-01-15 08:30:00",
      userCount: 12847,
      lastUpdated: new Date().toISOString(),
    },
    shards: { shards },
  };
}

function formatNumber(value) {
  return new Intl.NumberFormat("zh-CN").format(value ?? 0);
}

function renderStat(stat) {
  statSize.textContent = stat.blockchainSizeText || "--";
  statTx.textContent = formatNumber(stat.totalTransactions);
  statGenesis.textContent = stat.genesisBlockTime || "--";
  statUsers.textContent = formatNumber(stat.userCount);
}

function renderShards(shards) {
  shardList.innerHTML = "";

  if (!Array.isArray(shards) || shards.length === 0) {
    shardList.innerHTML = `<div class="empty-state">暂无分片区块数据。</div>`;
    return;
  }

  shards.forEach((shard) => {
    const panel = document.createElement("section");
    panel.className = "shard-panel";

    const header = document.createElement("div");
    header.className = "shard-header";
    header.innerHTML = `
      <div>
        <h3>${shard.label}</h3>
      </div>
      <div class="shard-summary">
        <span>区块数 ${formatNumber(shard.blockCount)}</span>
        <span>最新高度 #${formatNumber(shard.latestHeight)}</span>
        <span>最新哈希 ${shard.latestHash ? shard.latestHash.slice(0, 10) + "..." : "--"}</span>
      </div>
    `;

    const scroll = document.createElement("div");
    scroll.className = "chain-scroll";

    const row = document.createElement("div");
    row.className = "chain-row";

    shard.blocks.forEach((block, index) => {
      if (index > 0) {
        const connector = document.createElement("div");
        connector.className = "connector";
        row.appendChild(connector);
      }

      const node = blockTemplate.content.firstElementChild.cloneNode(true);
      node.classList.toggle("latest", Boolean(block.isLatest));
      node.querySelector(".block-id").textContent = `区块${block.blockId}`;
      node.querySelector(".block-hash").textContent = block.hashLast8 || block.hash.slice(-8) || "--";
      node.querySelector(".block-tx").textContent = `${formatNumber(block.txCount)} 笔交易`;
      node.querySelector(".block-time").textContent = block.timestamp || "Unavailable";
      row.appendChild(node);
    });

    if (shard.blocks.length === 0) {
      row.innerHTML = `<div class="empty-state">该分片当前未识别到区块记录。</div>`;
    }

    scroll.appendChild(row);
    panel.append(header, scroll);
    shardList.appendChild(panel);
  });
}

async function refresh() {
  try {
    const response = await fetch("/api/stat", { cache: "no-store" });
    if (!response.ok) throw new Error("API unavailable");
    const stat = await response.json();
    const shardResponse = await fetch("/api/blocks", { cache: "no-store" });
    if (!shardResponse.ok) throw new Error("API unavailable");
    const shardData = await shardResponse.json();
    const shards = (shardData.shards || []).map((shard) => ({
      ...shard,
      blocks: shard.blocks.map((block, i) => ({
        ...block,
        blockId: block.blockId || i + 1,
        hashLast8: block.hashLast8 || (block.hash || "").slice(-8),
      })),
    }));
    renderStat(stat);
    renderShards(shards);
  } catch {
    const mock = getMockData();
    renderStat(mock.stat);
    renderShards(mock.shards.shards);
  }
}

refresh();
