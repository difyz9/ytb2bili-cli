import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './style.css';
import { initAnalytics } from '../../utils/analytics';

// 初始化分析客户端
initAnalytics();

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
