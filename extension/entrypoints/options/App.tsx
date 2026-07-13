import React, { useState, useEffect } from 'react';
import { getBitableConfig, saveBitableConfig, clearBitableConfig } from '../../utils/bitable';

export default function App() {
  const [appId, setAppId] = useState('');
  const [appSecret, setAppSecret] = useState('');
  const [appToken, setAppToken] = useState('');
  const [tableId, setTableId] = useState('');
  const [saved, setSaved] = useState(false);
  const [loading, setLoading] = useState(true);
  const [testStatus, setTestStatus] = useState<'idle' | 'testing' | 'success' | 'error'>('idle');
  const [testMessage, setTestMessage] = useState('');

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    const config = await getBitableConfig();
    if (config) {
      setAppId(config.appId || '');
      setAppSecret(config.appSecret || '');
      setAppToken(config.appToken || '');
      setTableId(config.tableId || '');
    }
    setLoading(false);
  };

  const handleSave = async () => {
    await saveBitableConfig({
      appId: appId.trim(),
      appSecret: appSecret.trim(),
      appToken: appToken.trim(),
      tableId: tableId.trim(),
    });

    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  const handleClear = async () => {
    setAppId('');
    setAppSecret('');
    setAppToken('');
    setTableId('');
    await clearBitableConfig();
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  const handleTestConnection = async () => {
    setTestStatus('testing');
    setTestMessage('');

    try {
      if (!appId || !appSecret) {
        setTestStatus('error');
        const msg = '请先填写 App ID 和 App Secret';
        setTestMessage(msg);
        return;
      }

      if (!appToken || !tableId) {
        setTestStatus('error');
        const msg = '请先填写 App Token 和 Table ID';
        setTestMessage(msg);
        return;
      }

      // 测试获取 access token
      const tokenResponse = await fetch('https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          app_id: appId,
          app_secret: appSecret,
        }),
      });

      const tokenData = await tokenResponse.json();
      if (tokenData.code !== 0) {
        setTestStatus('error');
        const msg = '获取 Token 失败: ' + (tokenData.msg || 'unknown error');
        setTestMessage(msg);
        return;
      }

      // 测试读取多维表格
      const bearerPrefix = 'Bearer ';
      const authHeader = bearerPrefix + tokenData.tenant_access_token;
      const bitableUrl = 'https://open.feishu.cn/open-apis/bitable/v1/apps/' + appToken + '/tables/' + tableId + '/records?page_size=1';
      const testResponse = await fetch(bitableUrl, {
        method: 'GET',
        headers: {
          'Authorization': authHeader,
        },
      });

      const testData = await testResponse.json();
      if (testData.code === 0) {
        setTestStatus('success');
        const msg = '连接成功！多维表格可访问';
        setTestMessage(msg);
      } else {
        setTestStatus('error');
        const msg = '读取失败: ' + (testData.msg || 'unknown error');
        setTestMessage(msg);
      }
    } catch (error) {
      setTestStatus('error');
      const msg = '连接失败: ' + (error instanceof Error ? error.message : String(error));
      setTestMessage(msg);
    }
  };

  if (loading) {
    return <div className="container">加载中...</div>;
  }

  return (
    <div className="container">
      <h1>Ytb2Bili 设置</h1>
      
      {/* 飞书多维表格配置 */}
      <section className="section">
        <h2>飞书多维表格</h2>
        <p className="description">
          配置飞书应用和多维表格信息，用于提交 YouTube 视频数据到多维表格。
        </p>
        
        <div className="form-group">
          <label>App ID</label>
          <input
            type="text"
            value={appId}
            onChange={(e) => setAppId(e.target.value)}
            placeholder="cli_xxxxxxxxxx"
            className="input"
          />
          <p className="hint">
            在飞书开放平台创建应用后获取
          </p>
        </div>

        <div className="form-group">
          <label>App Secret</label>
          <input
            type="password"
            value={appSecret}
            onChange={(e) => setAppSecret(e.target.value)}
            placeholder="xxxxxxxxxxxxxxxx"
            className="input"
          />
          <p className="hint">
            在飞书开放平台创建应用后获取
          </p>
        </div>

        <div className="form-group">
          <label>App Token (多维表格)</label>
          <input
            type="text"
            value={appToken}
            onChange={(e) => setAppToken(e.target.value)}
            placeholder="MEGxxxxxx"
            className="input"
          />
          <p className="hint">
            多维表格 URL 中 /apps/ 后面的字符串
          </p>
        </div>

        <div className="form-group">
          <label>Table ID</label>
          <input
            type="text"
            value={tableId}
            onChange={(e) => setTableId(e.target.value)}
            placeholder="tblxxxxxx"
            className="input"
          />
          <p className="hint">
            多维表格 URL 中 /tables/ 后面的字符串
          </p>
        </div>

        <button onClick={handleTestConnection} className="btn btn-secondary" disabled={testStatus === 'testing'}>
          {testStatus === 'testing' ? '测试中...' : '测试连接'}
        </button>
        
        {testMessage && (
          <div className={'test-result ' + testStatus}>
            {testMessage}
          </div>
        )}
      </section>

      {/* 保存按钮 */}
      <div className="button-group">
        <button onClick={handleSave} className="btn btn-primary">
          {saved ? '已保存' : '保存设置'}
        </button>
        <button onClick={handleClear} className="btn btn-secondary">
          清除配置
        </button>
      </div>

      {/* 使用说明 */}
      <section className="section">
        <h2>使用说明</h2>
        <div className="steps">
          <div className="step">
            <span className="step-number">1</span>
            <div className="step-content">
              <strong>启动后端服务</strong>
              <code>y2b server --addr :8096</code>
            </div>
          </div>
          <div className="step">
            <span className="step-number">2</span>
            <div className="step-content">
              <strong>配置飞书应用</strong>
              <p>填写 App ID、App Secret、App Token 和 Table ID</p>
            </div>
          </div>
          <div className="step">
            <span className="step-number">3</span>
            <div className="step-content">
              <strong>访问 YouTube</strong>
              <p>打开任意 YouTube 视频页面</p>
            </div>
          </div>
          <div className="step">
            <span className="step-number">4</span>
            <div className="step-content">
              <strong>点击插件按钮</strong>
              <p>点击播放器上的提交按钮</p>
            </div>
          </div>
        </div>
      </section>

      {/* 相关链接 */}
      <section className="section">
        <h2>相关链接</h2>
        <ul>
          <li>
            <a href="https://gitee.com/difyz/ytb2bili-go" target="_blank" rel="noopener noreferrer">
              ytb2bili-go 后端项目
            </a>
          </li>
          <li>
            <a href="https://gitee.com/difyz/ytb2bili-extension" target="_blank" rel="noopener noreferrer">
              浏览器插件项目
            </a>
          </li>
          <li>
            <a href="https://open.feishu.cn/document/home/introduction-to-custom-bot" target="_blank" rel="noopener noreferrer">
              飞书开放平台文档
            </a>
          </li>
        </ul>
      </section>
    </div>
  );
}
