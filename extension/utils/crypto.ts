/**
 * Cookies 加密工具
 * 使用 AES-GCM 加密 cookies 数据
 */

/**
 * 将字符串转换为 ArrayBuffer
 */
function str2ab(str: string): ArrayBuffer {
  const encoder = new TextEncoder();
  return encoder.encode(str).buffer as ArrayBuffer;
}

/**
 * 将 ArrayBuffer 转换为字符串
 */
function ab2str(buf: ArrayBuffer): string {
  const decoder = new TextDecoder();
  return decoder.decode(buf);
}

/**
 * 将 ArrayBuffer 转换为 Base64
 */
function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

/**
 * 将 Base64 转换为 ArrayBuffer
 */
function base64ToArrayBuffer(base64: string): ArrayBuffer {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

/**
 * 从密钥字符串生成 CryptoKey
 */
async function getKey(keyStr: string): Promise<CryptoKey> {
  const keyData = str2ab(keyStr.padEnd(32, '0').slice(0, 32));
  return await crypto.subtle.importKey(
    'raw',
    keyData,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt']
  );
}

/**
 * 使用 AES-GCM 加密数据
 * @param data 要加密的数据（字符串）
 * @param keyStr 加密密钥（字符串）
 * @returns Base64 编码的加密数据（包含 IV）
 */
export async function encryptData(data: string, keyStr: string): Promise<string> {
  try {
    const key = await getKey(keyStr);
    
    // 生成随机 IV
    const iv = crypto.getRandomValues(new Uint8Array(12));
    
    // 加密数据
    const encrypted = await crypto.subtle.encrypt(
      { name: 'AES-GCM', iv: iv },
      key,
      str2ab(data)
    );
    
    // 组合 IV 和加密数据
    const combined = new Uint8Array(iv.length + encrypted.byteLength);
    combined.set(iv, 0);
    combined.set(new Uint8Array(encrypted), iv.length);
    
    // 转换为 Base64
    return arrayBufferToBase64(combined.buffer);
  } catch (error) {
    console.error('[Crypto] Encryption failed:', error);
    throw new Error('加密失败');
  }
}

/**
 * 使用 AES-GCM 解密数据
 * @param encryptedBase64 Base64 编码的加密数据（包含 IV）
 * @param keyStr 解密密钥（字符串）
 * @returns 解密后的字符串
 */
export async function decryptData(encryptedBase64: string, keyStr: string): Promise<string> {
  try {
    const key = await getKey(keyStr);
    
    // 解析 Base64
    const combined = base64ToArrayBuffer(encryptedBase64);
    
    // 提取 IV 和加密数据
    const iv = combined.slice(0, 12);
    const encrypted = combined.slice(12);
    
    // 解密数据
    const decrypted = await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: iv },
      key,
      encrypted
    );
    
    return ab2str(decrypted);
  } catch (error) {
    console.error('[Crypto] Decryption failed:', error);
    throw new Error('解密失败');
  }
}

/**
 * 生成随机 nonce
 * @param length nonce 长度
 * @returns 随机字符串
 */
export function generateNonce(length: number = 16): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

/**
 * 生成签名（符合 go-auth 规范）
 * @param params 签名参数 { appId, timestamp, nonce }
 * @param secret 签名密钥
 * @returns 十六进制格式的签名字符串
 */
export async function generateSignature(params: Record<string, string>, secret: string): Promise<string> {
  // 1. 按 key 排序
  const sortedKeys = Object.keys(params).sort();
  
  // 2. 拼接成 key1=value1&key2=value2 格式
  const signString = sortedKeys.map(key => `${key}=${params[key]}`).join('&');
  
  // 3. HMAC-SHA256 签名
  const encoder = new TextEncoder();
  const keyData = encoder.encode(secret);
  const dataBuffer = encoder.encode(signString);
  
  const key = await crypto.subtle.importKey(
    'raw',
    keyData,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign']
  );
  
  const signature = await crypto.subtle.sign('HMAC', key, dataBuffer);
  
  // 4. 转换为十六进制字符串（与 go-auth 保持一致）
  const hashArray = Array.from(new Uint8Array(signature));
  const hexString = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
  
  return hexString;
}
