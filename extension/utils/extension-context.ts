const EXTENSION_CONTEXT_INVALIDATED_RE = /Extension context invalidated/i;

export function isExtensionContextInvalidated(error: unknown): boolean {
  if (!error) {
    return false;
  }

  if (typeof error === 'string') {
    return EXTENSION_CONTEXT_INVALIDATED_RE.test(error);
  }

  if (error instanceof Error) {
    return EXTENSION_CONTEXT_INVALIDATED_RE.test(error.message);
  }

  const maybeMessage = (error as { message?: unknown }).message;
  return typeof maybeMessage === 'string' && EXTENSION_CONTEXT_INVALIDATED_RE.test(maybeMessage);
}

export function getExtensionContextInvalidatedMessage(): string {
  return '扩展上下文已失效，通常是扩展刚重载或更新。请关闭当前弹窗或页面后重新打开再试。';
}
