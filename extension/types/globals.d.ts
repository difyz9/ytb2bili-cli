// 全局类型定义
/// <reference types="@types/webextension-polyfill" />

// 确保 browser API 在全局可用
declare const browser: typeof import('webextension-polyfill');

// WXT 框架自动注入的全局函数
declare function defineBackground(main: () => void): any;
declare function defineContentScript(config: any): any;
declare function defineUnlistedScript(main: () => void): any;

// 环境变量类型定义
interface ImportMetaEnv {
  readonly VITE_ANALYTICS_SERVER_URL?: string;
  readonly VITE_ANALYTICS_PRODUCT_NAME?: string;
  readonly VITE_ANALYTICS_DEBUG?: string;
  readonly VITE_APP_ID?: string;
  readonly VITE_APP_SECRET?: string;
  readonly VITE_BACKEND_URL?: string;
	readonly VITE_YTB2BILI_SERVER_TOKEN?: string;
  readonly VITE_COOKIES_ENCRYPT_KEY?: string;
  readonly VITE_MEMBERSHIP_URL?: string;
  readonly VITE_PROJECT_ID?: string;
  readonly VITE_WEB_APP_URL?: string;
  readonly DEV?: boolean;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
