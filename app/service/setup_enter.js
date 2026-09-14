const status = document.getElementById('status');
const retry = document.getElementById('retry');
let active = true;
window.addEventListener('pagehide', () => { active = false; });
async function waitForReady() {
  retry.hidden = true;
  status.textContent = '配置已保存，正在等待数据库和应用就绪…';
  const deadline = Date.now() + 90000;
  while (active && Date.now() < deadline) {
    try {
      const response = await fetch('/api/install/check', { cache: 'no-store', signal: AbortSignal.timeout(3000) });
      const result = await response.json();
      if (response.ok && result.code === 0 && result.data.ready) {
        window.location.replace('/admin/');
        return;
      }
    } catch { /* The process briefly disconnects while restarting. */ }
    await new Promise(resolve => setTimeout(resolve, 1500));
  }
  if (active) {
    status.textContent = '等待启动超时，请检查服务日志后重新检查。配置已经保存，无需重复安装。';
    retry.hidden = false;
  }
}
retry.addEventListener('click', waitForReady);
waitForReady();
