const iam = require('../../utils/iam');
Page({
  data: {
    config: null, state: '', busy: false, error: '', bindForm: false,
    username: '', password: '', email: '', realName: '', nickname: '',
    accepted: false, challenge: null, methods: [], methodIndex: 0, factorCode: '',
    authenticated: false, notice: '', agreementContent: '',
  },
  onLoad() {
    this.run(async () => {
      const config = await iam.config();
      this.setData({ config, authenticated: Boolean(iam.tokens()) });
    });
  },
  async run(action) {
    if (this.data.busy) return;
    this.setData({ busy: true, error: '', notice: '' });
    try { await action(); } catch (err) { this.setData({ error: err.message || '操作失败，请重试' }); }
    finally { this.setData({ busy: false }); }
  },
  apply(result) {
    const challenge = result.twoFactorChallenge || null;
    const names = { password: '账号密码', phone: '短信验证码', email: '邮箱验证码', totp: '验证器动态码', recovery: '恢复码' };
    const methods = challenge ? challenge.methods.filter((m) => m.available).map((m) => ({ ...m, label: names[m.method] || m.method })) : [];
    this.setData({ state: result.state, challenge, methods, methodIndex: 0, bindForm: result.state === 'bind_required', authenticated: result.state === 'authenticated', password: '', factorCode: '' });
  },
  login() { this.run(async () => this.apply(await iam.login())); },
  phone(event) {
    if (!event.detail.code) { this.setData({ error: '未授权手机号。可以重试或选择绑定已有账号。' }); return; }
    this.run(async () => this.apply(await iam.native('phone', { phoneCode: event.detail.code })));
  },
  field(event) { this.setData({ [event.currentTarget.dataset.field]: event.detail.value }); },
  showBind() { this.setData({ bindForm: true }); },
  bind() { this.run(async () => this.apply(await iam.native('bind', { username: this.data.username, password: this.data.password })));
  },
  accept(event) { this.setData({ accepted: event.detail.value.includes('yes') }); },
  complete() {
    if (!this.data.accepted) { this.setData({ error: '请阅读协议并确认授权' }); return; }
    this.run(async () => this.apply(await iam.native('complete', {
      approveScopes: true,
      acceptedAgreementKeys: this.data.config.agreements.map((a) => a.key),
      profile: { email: this.data.email, realName: this.data.realName, nickname: this.data.nickname },
    })));
  },
  method(event) { this.setData({ methodIndex: Number(event.detail.value), factorCode: '' }); },
  sendCode() { this.run(async () => {
    const selected = this.data.methods[this.data.methodIndex];
    await iam.native('two-factor/send', { method: selected.method });
    this.setData({ notice: '验证码已发送，请稍候查收。' });
  }); },
  verify() { this.run(async () => this.apply(await iam.native('two-factor/verify', {
    method: this.data.methods[this.data.methodIndex].method, code: this.data.factorCode,
  })));
  },
  refresh() { this.run(async () => { await iam.refresh(); this.setData({ notice: '令牌已刷新。' }); }); },
  logout() { this.run(async () => { await iam.logout(); this.setData({ authenticated: false, state: '', notice: '已退出。' }); }); },
  readAgreement(event) { this.run(async () => {
    const link = this.data.config.agreements[event.currentTarget.dataset.index];
    const result = await iam.agreement(link);
    this.setData({ agreementContent: result.contentMd || result.content || '', agreementUrl: result.contentUrl || link.url });
  }); },
  copyAgreementURL() { wx.setClipboardData({ data: this.data.agreementUrl }); },
  closeAgreement() { this.setData({ agreementContent: '', agreementUrl: '' }); },
});
