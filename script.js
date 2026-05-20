const statSize = document.querySelector("#stat-size");
const statTx = document.querySelector("#stat-tx");
const statUsers = document.querySelector("#stat-users");
const shardList = document.querySelector("#shard-list");
const blockTemplate = document.querySelector("#block-template");
const modalOverlay = document.querySelector("#modal-overlay");
const modalClose = document.querySelector("#modal-close");
const modalTitle = document.querySelector("#modal-title");
const modalBadges = document.querySelector("#modal-badges");
const modalMeta = document.querySelector("#modal-meta");
const txEmpty = document.querySelector("#tx-empty");
const txTableWrap = document.querySelector("#tx-table-wrap");
const txTableBody = document.querySelector("#tx-table-body");
const composeTrigger = document.querySelector("#compose-trigger");
const composeOverlay = document.querySelector("#compose-overlay");
const composeClose = document.querySelector("#compose-close");
const composeCancel = document.querySelector("#compose-cancel");
const composeForm = document.querySelector("#compose-form");
const composeFeedback = document.querySelector("#compose-feedback");
const composeSubmit = document.querySelector("#compose-submit");
const senderInput = document.querySelector("#tx-sender");
const recipientInput = document.querySelector("#tx-recipient");
const valueInput = document.querySelector("#tx-value");

const TEXT = {
  loading: "正在加载交易信息...",
  noTransactions: "该区块当前没有交易记录。",
  loadFailed: "交易信息读取失败。",
  latestBlock: "最新区块",
  historyBlock: "历史区块",
  transactionSuffix: "笔交易",
};

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

function shortHash(hash) {
  if (!hash) return "--";
  if (hash.length <= 18) return hash;
  return `${hash.slice(0, 10)}...${hash.slice(-8)}`;
}

function renderStat(stat) {
  statSize.textContent = stat.blockchainSizeText || "--";
  statTx.textContent = formatNumber(stat.totalTransactions);
  statUsers.textContent = formatNumber(stat.userCount);
}

function openComposeModal() {
  composeFeedback.textContent = "";
  composeFeedback.className = "compose-feedback hidden";
  composeForm.reset();
  composeOverlay.classList.remove("hidden");
  composeOverlay.setAttribute("aria-hidden", "false");
  document.body.classList.add("modal-open");
  setTimeout(() => senderInput.focus(), 0);
}

function closeComposeModal() {
  composeOverlay.classList.add("hidden");
  composeOverlay.setAttribute("aria-hidden", "true");
  if (modalOverlay.classList.contains("hidden")) {
    document.body.classList.remove("modal-open");
  }
}

function setComposeFeedback(message, type) {
  composeFeedback.textContent = message;
  composeFeedback.className = `compose-feedback ${type}`;
}

function closeModal() {
  modalOverlay.classList.add("hidden");
  modalOverlay.setAttribute("aria-hidden", "true");
  document.body.classList.remove("modal-open");
}

function showModal(shard, block) {
  modalTitle.textContent = `${shard.label} - 区块 ${block.height ?? block.blockId ?? "--"}`;
  modalBadges.innerHTML = `
    <span class="modal-badge">${block.isLatest ? TEXT.latestBlock : TEXT.historyBlock}</span>
    <span class="modal-badge">${formatNumber(block.txCount)} ${TEXT.transactionSuffix}</span>
  `;

  modalMeta.innerHTML = `
    <article class="meta-card">
      <span>区块高度</span>
      <strong>#${formatNumber(block.height ?? 0)}</strong>
    </article>
    <article class="meta-card">
      <span>区块哈希</span>
      <strong title="${block.hash || ""}">${block.hash || "--"}</strong>
    </article>
    <article class="meta-card">
      <span>父区块哈希</span>
      <strong title="${block.parentHash || ""}">${block.parentHash || "--"}</strong>
    </article>
    <article class="meta-card">
      <span>区块时间</span>
      <strong>${block.timestamp || "Unavailable"}</strong>
    </article>
  `;

  txEmpty.textContent = TEXT.loading;
  txEmpty.classList.remove("hidden");
  txTableWrap.classList.add("hidden");
  txTableBody.innerHTML = "";

  modalOverlay.classList.remove("hidden");
  modalOverlay.setAttribute("aria-hidden", "false");
  document.body.classList.add("modal-open");

  loadTransactions(shard.id, block.hash);
}

async function loadTransactions(shardID, hash) {
  try {
    const response = await fetch(
      `/api/block-transactions?shard=${encodeURIComponent(shardID)}&hash=${encodeURIComponent(hash)}`,
      { cache: "no-store" }
    );
    if (!response.ok) throw new Error("API unavailable");
    const data = await response.json();
    renderTransactions(data.transactions || []);
  } catch {
    txEmpty.textContent = TEXT.loadFailed;
    txEmpty.classList.remove("hidden");
    txTableWrap.classList.add("hidden");
    txTableBody.innerHTML = "";
  }
}

async function submitTransaction(event) {
  event.preventDefault();

  const payload = {
    sender: senderInput.value.trim(),
    recipient: recipientInput.value.trim(),
    value: valueInput.value.trim(),
  };

  if (!payload.sender || !payload.recipient || !payload.value) {
    setComposeFeedback("请完整填写发送方、接收方和转账金额。", "error");
    return;
  }

  composeSubmit.disabled = true;
  setComposeFeedback("正在写入 Transactions.csv ...", "pending");

  try {
    const response = await fetch("/api/transactions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || "写入失败");
    }
    setComposeFeedback(`写入成功，时间戳 ${data.timestamp}。`, "success");
    setTimeout(() => closeComposeModal(), 900);
  } catch (error) {
    setComposeFeedback(error.message || "交易写入失败。", "error");
  } finally {
    composeSubmit.disabled = false;
  }
}

function renderTransactions(transactions) {
  if (!Array.isArray(transactions) || transactions.length === 0) {
    txEmpty.textContent = TEXT.noTransactions;
    txEmpty.classList.remove("hidden");
    txTableWrap.classList.add("hidden");
    txTableBody.innerHTML = "";
    return;
  }

  txEmpty.classList.add("hidden");
  txTableWrap.classList.remove("hidden");
  txTableBody.innerHTML = transactions
    .map(
      (tx) => `
        <tr>
          <td>${formatNumber(tx.index)}</td>
          <td title="${tx.sender || ""}">${tx.sender || ""}</td>
          <td title="${tx.recipient || ""}">${tx.recipient || ""}</td>
          <td>${tx.value || ""}</td>
          <td>${formatNumber(tx.nonce)}</td>
          <td title="${tx.txHash || ""}">${shortHash(tx.txHash)}</td>
        </tr>
      `
    )
    .join("");
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
      node.tabIndex = 0;
      node.setAttribute("role", "button");
      node.setAttribute("aria-label", `查看 ${shard.label} 区块 ${block.height ?? block.blockId} 详情`);
      node.querySelector(".block-id").textContent = `区块${block.blockId}`;
      node.querySelector(".block-hash").textContent = block.hashLast8 || block.hash.slice(-8) || "--";
      node.querySelector(".block-tx").textContent = `${formatNumber(block.txCount)} 笔交易`;
      node.querySelector(".block-time").textContent = block.timestamp || "Unavailable";
      node.addEventListener("click", () => showModal(shard, block));
      node.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          showModal(shard, block);
        }
      });
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

modalClose.addEventListener("click", closeModal);
modalOverlay.addEventListener("click", (event) => {
  if (event.target === modalOverlay) {
    closeModal();
  }
});
composeTrigger.addEventListener("click", openComposeModal);
composeClose.addEventListener("click", closeComposeModal);
composeCancel.addEventListener("click", closeComposeModal);
composeOverlay.addEventListener("click", (event) => {
  if (event.target === composeOverlay) {
    closeComposeModal();
  }
});
composeForm.addEventListener("submit", submitTransaction);

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && !modalOverlay.classList.contains("hidden")) {
    closeModal();
  }
  if (event.key === "Escape" && !composeOverlay.classList.contains("hidden")) {
    closeComposeModal();
  }
});

refresh();
