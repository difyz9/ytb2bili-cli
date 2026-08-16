#!/usr/bin/env python3
"""
Pay Skill Handler — 文档分析付费调用示例

本 handler 演示 Pay Skill 的两种集成模式：
  模式 A：直连模式 — 调用商户后端 API（适合独立部署）
  模式 B：内嵌模式 — 依赖 merchant_server.py（适合一体化）

接口规范：
  输入: {"query": "文档路径或查询", "epub_path": "文档路径（可选）"}
  输出:
    - 成功: {"success": true, "content": "付费内容"}
    - 需付费: {"success": false, "payment_required": true, "payment_code": "...",
               "out_trade_no": "..."}
    - 失败: {"success": false, "error": "错误信息"}
"""

import json
import os
import sys
import urllib.request
import urllib.error

# ======== 配置 ========

# 商户后端地址（模式 A 使用）
MERCHANT_URL = os.getenv("MERCHANT_URL", "http://localhost:8080/api/resource")


# ======== 模式 A：直连商户后端 ========

def pay_handler(input_data):
    """
    SkillHub Pay Skill 入口函数。
    由 SkillHub 在用户触发 Skill 时调用。
    """
    query = input_data.get('query', '') or input_data.get('epub_path', '')
    if not query:
        return {
            'success': False,
            'error': '请提供文档路径或查询内容',
            'message': '缺少参数: query 或 epub_path',
        }

    # 检查是否已有支付令牌（从 input_data 中传入）
    out_trade_no = input_data.get('out_trade_no', '')
    payment_code = input_data.get('payment_code', '')

    if out_trade_no:
        # 已有订单号 → 验证支付并获取内容
        return _verify_and_get(query, out_trade_no, payment_code)
    else:
        # 首次调用 → 请求支付
        return _request_payment(query)


def _request_payment(query):
    """首次调用：向商户后端请求支付"""
    try:
        req = urllib.request.Request(
            MERCHANT_URL,
            data=json.dumps({"query": query}).encode('utf-8'),
            headers={'Content-Type': 'application/json'},
            method='POST'
        )
        resp = urllib.request.urlopen(req, timeout=10)
        body = json.loads(resp.read().decode('utf-8'))

        # HTTP 200 = 无需付费（可能已有免费额度）
        return {
            'success': True,
            'content': body.get('content', ''),
        }

    except urllib.error.HTTPError as e:
        if e.code == 402:
            # 需要支付
            body = json.loads(e.read().decode('utf-8'))
            payment_code = e.headers.get('WeixinPay-Required', '')
            out_trade_no = e.headers.get('X-Out-Trade-No', '')

            return {
                'success': False,
                'payment_required': True,
                'payment_code': payment_code,
                'out_trade_no': out_trade_no,
                'amount': body.get('amount', '0.50'),
                'message': body.get('message', '需要支付后才能获取内容'),
            }
        else:
            return {
                'success': False,
                'error': f'商户后端错误 (HTTP {e.code})',
                'message': str(e),
            }

    except urllib.error.URLError as e:
        return {
            'success': False,
            'error': f'无法连接商户后端: {e.reason}',
            'message': '请确认商户后端已启动 (python3 merchant_server.py)',
        }


def _verify_and_get(query, out_trade_no, payment_code):
    """支付后重试：验证支付状态并获取付费内容"""
    try:
        req = urllib.request.Request(
            MERCHANT_URL,
            data=json.dumps({"query": query}).encode('utf-8'),
            headers={
                'Content-Type': 'application/json',
                'X-Out-Trade-No': out_trade_no,
                'WeixinPay-Required': payment_code,
            },
            method='POST'
        )
        resp = urllib.request.urlopen(req, timeout=10)
        body = json.loads(resp.read().decode('utf-8'))

        code = body.get('code', '')

        if code == 'SUCCESS':
            return {
                'success': True,
                'content': body.get('content', ''),
                'already_fulfilled': body.get('already_fulfilled', False),
            }
        elif code == 'REFUNDED':
            return {
                'success': False,
                'refunded': True,
                'message': body.get('message', '服务异常，已自动退款'),
            }
        elif code == 'NOT_PAID':
            return {
                'success': False,
                'payment_required': True,
                'message': body.get('message', '支付尚未完成，请稍后重试'),
            }
        else:
            return {
                'success': False,
                'error': f'未知状态: {code}',
                'message': body.get('message', ''),
            }

    except urllib.error.HTTPError as e:
        if e.code == 402:
            return {
                'success': False,
                'payment_required': True,
                'message': '支付尚未完成',
            }
        return {
            'success': False,
            'error': f'商户后端错误 (HTTP {e.code})',
        }

    except urllib.error.URLError as e:
        return {
            'success': False,
            'error': f'无法连接商户后端: {e.reason}',
        }


# ======== 模式 B：内嵌模式（一体化测试用）========

def embedded_handler(input_data):
    """
    内嵌模式 handler — 不依赖外部商户后端。
    需要在启动时同时运行 merchant_server.py（Mock 模式）。
    此 handler 作为 BUSINESS_HANDLER 注入。
    """
    return pay_handler(input_data)


def execute_business(query):
    """
    业务逻辑函数 —— 可注入 merchant_server.py 作为 BUSINESS_HANDLER。
    生产环境替换为真实业务（如文档分析、AI 生成等）。

    返回: (success: bool, content: str, error: str)
    """
    # 模拟：正常情况下返回付费内容
    # query 可以是文档路径，这里做模拟分析
    if not query:
        return False, '', '查询内容为空'

    # 模拟业务处理
    content = (
        f"【付费分析报告】\n"
        f"━━━━━━━━━━━━━━━━━━━━━━\n"
        f"查询内容: {query}\n"
        f"━━━━━━━━━━━━━━━━━━━━━━\n\n"
        f"📊 分析结果\n"
        f"  总字数：约 12,450 字\n"
        f"  主题提取：3 个核心议题\n"
        f"  关键实体识别：15 个\n\n"
        f"📝 智能摘要\n"
        f"  本文档主要讨论了相关领域的技术方案和实施策略。\n"
        f"  作者提出了三个关键创新点，并给出了详细的实施方案。\n\n"
        f"💡 建议\n"
        f"  1. 关注文档中提到的性能优化方案\n"
        f"  2. 核心技术方案值得深入研究和验证\n"
        f"  3. 注意不同方案间的兼容性\n\n"
        f"━━━━━━━━━━━━━━━━━━━━━━\n"
        f"本次调用费用：¥0.50\n"
        f"支付方式：微信 AI 专属卡（X402 协议）\n"
        f"━━━━━━━━━━━━━━━━━━━━━━"
    )

    return True, content, None


# ======== CLI 测试入口 ========

def main():
    """CLI 测试入口"""
    if len(sys.argv) >= 2 and sys.argv[1] == '--handler':
        input_data = json.loads(sys.stdin.read())
        result = pay_handler(input_data)
        print(json.dumps(result, ensure_ascii=False))
        return

    # 测试模式：与 merchant_server.py 配合
    print("Pay Skill Handler 测试工具")
    print("==========================")
    print()
    print("用法:")
    print("  # 测试 pay_handler")
    print('  echo \'{"query": "test.epub"}\' | python3 handler.py --handler')
    print()
    print("  # 启动商户后端 + 注入本 handler 的业务逻辑")
    print("  python3 -c \"")
    print("  from merchant_server import set_business_handler, main as ms_main")
    print("  from handler import execute_business")
    print("  set_business_handler(execute_business)")
    print("  ms_main()")
    print('  "')
    print()


if __name__ == '__main__':
    main()
