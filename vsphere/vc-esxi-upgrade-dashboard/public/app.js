function fmtPct(x){
  return `${Number(x).toFixed(1)}%`;
}

function healthFromPct(p){
  if (p >= 99.9) return {label:"Healthy", cls:"good"};
  if (p >= 85) return {label:"In progress", cls:"warn"};
  return {label:"At risk", cls:"bad"};
}

function addLog(msg){
  const log = document.getElementById("activityLog");
  const item = document.createElement("div");
  item.className = "log-item";
  const ts = new Date().toLocaleString();
  item.innerHTML = `<div class="log-title">${msg}</div><div class="log-meta">${ts}</div>`;
  log.prepend(item);

  while (log.children.length > 8) log.removeChild(log.lastChild);
}

async function loadData(){
  const res = await fetch("/api/v1/vcenters", {cache:"no-store"});
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();

  document.getElementById("generatedAt").textContent = data.generated_at;
  document.getElementById("targetVersion").textContent = data.target_esxi_version;

  const rows = data.rows || [];

  const totalVCs = rows.length;
  const totalHosts = rows.reduce((a,r)=>a + (r.total_hosts||0), 0);
  const totalUpgraded = rows.reduce((a,r)=>a + (r.upgrade_completed_total||0), 0);
  const weightedPct = totalHosts ? (totalUpgraded/totalHosts*100) : 0;

  document.getElementById("kpiVcenters").textContent = totalVCs;
  document.getElementById("kpiHosts").textContent = totalHosts;
  document.getElementById("kpiUpgraded").textContent = totalUpgraded;
  document.getElementById("kpiCompletion").textContent = fmtPct(weightedPct);

  const health = healthFromPct(weightedPct);
  document.getElementById("healthPill").textContent = health.label;

  const tbody = document.getElementById("vcTableBody");
  tbody.innerHTML = "";

  for (const r of rows){
    const pct = Number(r.completion_percentage || 0);
    const tag = healthFromPct(pct);

    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>
        <span class="tag"><span class="bullet ${tag.cls}"></span>${r.vcenter}</span>
      </td>
      <td class="num">${r.total_hosts}</td>
      <td class="num">${r.upgrade_completed_total}</td>
      <td>
        <div class="progress-cell">
          <div class="progress-wrap"><div class="progress-bar"></div></div>
          <div class="progress-label">${fmtPct(pct)}</div>
        </div>
      </td>
    `;
    tbody.appendChild(tr);

    requestAnimationFrame(() => {
      tr.querySelector(".progress-bar").style.width = `${pct}%`;
    });
  }

  addLog(`Refreshed ${totalVCs} vCenters; overall completion ${fmtPct(weightedPct)}`);
}

async function refresh(){
  try{
    await loadData();
  }catch(e){
    addLog(`Refresh failed: ${e.message}`);
    document.getElementById("healthPill").textContent = "Error";
  }
}

// Export dashboard as PNG screenshot (preserves all styling including gradients)
async function exportScreenshot(){
  // Load html2canvas library from CDN
  const script = document.createElement("script");
  script.src = "https://html2canvas.hertzen.com/dist/html2canvas.min.js";
  script.onload = async () => {
    const btn = document.getElementById("exportBtn");
    btn.textContent = "Exporting...";
    
    try {
      const canvas = await html2canvas(document.body, {
        backgroundColor: null,
        scale: 2,
        logging: false,
        allowTaint: true,
        useCORS: true,
      });
      
      const link = document.createElement("a");
      link.href = canvas.toDataURL("image/png");
      link.download = `esxi-dashboard-${new Date().toISOString().split('T')[0]}.png`;
      link.click();
      
      btn.textContent = "Export PNG";
      addLog("Dashboard exported as PNG");
    } catch (e) {
      addLog(`Export failed: ${e.message}`);
      btn.textContent = "Export PNG";
    }
  };
  document.head.appendChild(script);
}

document.getElementById("refreshBtn").addEventListener("click", refresh);
document.getElementById("exportBtn").addEventListener("click", exportScreenshot);

refresh();
setInterval(refresh, 60_000);