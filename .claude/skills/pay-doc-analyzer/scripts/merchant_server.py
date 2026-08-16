#!/usr/bin/env python3
"""
Pay Skill Merchant Backend — Python
Mock mode (default) for local testing | Production mode for real X402
"""

import http.server, json, os, sys, uuid, argparse
from datetime import datetime

MOCK_MODE = True
BUSINESS_HANDLER = None

def set_business_handler(fn):
    global BUSINESS_HANDLER; BUSINESS_HANDLER = fn

class OrderStore:
    def __init__(self):
        self._orders = {}
    def save(self, o):
        self._orders[o['out_trade_no']] = o
    def get(self, k):
        return self._orders.get(k)

store = OrderStore()

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/health':
            self._json(200, {"status":"ok","mode":"MOCK" if MOCK_MODE else "PROD"})
        else:
            self._json(404,{"error":"not found"})

    def do_POST(self):
        length = int(self.headers.get('Content-Length',0))
        body = self.rfile.read(length) if length else b'{}'
        try:
            data = json.loads(body)
        except json.JSONDecodeError:
            self._json(400,{"error":"invalid json"}); return
        if self.path == '/api/resource':
            self._handle_resource(data)
        elif self.path == '/api/pay/notify':
            self._handle_notify(data,'PAID')
        elif self.path == '/api/refund/notify':
            self._handle_notify(data,'REFUNDED')
        else:
            self._json(404,{"error":"not found"})

    def _handle_resource(self, data):
        q = data.get('query','') or data.get('epub_path','')
        oid = self.headers.get('X-Out-Trade-No')
        if oid:
            self._verify_and_fulfill(oid,q)
        else:
            self._create_order(q)

    def _create_order(self, query):
        oid = f"PAY_{datetime.now().strftime('%Y%m%d%H%M%S')}_{uuid.uuid4().hex[:12]}"
        pc = f"MOCK_PAY_{uuid.uuid4().hex[:16].upper()}"
        store.save({'out_trade_no':oid,'payment_code':pc,'amount':50,
                     'query':query,'status':'INIT',
                     'created_at':datetime.now().isoformat()})
        self.send_response(402)
        self.send_header('Content-Type','application/json; charset=utf-8')
        self.send_header('WeixinPay-Required',pc)
        self.send_header('X-Out-Trade-No',oid)
        self.end_headers()
        self.wfile.write(json.dumps({
            'code':'PAYMENT_REQUIRED','message':'需支付后获取内容',
            'WeixinPay':{'WeixinPay-Required':pc,'prompt':'请完成支付授权'},
            'out_trade_no':oid,'amount':'0.50','currency':'CNY',
        },ensure_ascii=False).encode())

    def _verify_and_fulfill(self, oid, query):
        order = store.get(oid)
        if not order:
            self._json(404,{"error":f"订单不存在:{oid}"}); return
        if order.get('status') == 'FULFILLED':
            self._json(200,{'code':'SUCCESS','message':'已缓存',
                           'out_trade_no':oid,'content':order['result'],
                           'already_fulfilled':True}); return
        # Mock: auto-success
        biz_q = query or order['query']
        if BUSINESS_HANDLER:
            ok, content, err = BUSINESS_HANDLER(biz_q)
        else:
            ok, content, err = True, _gen_content(biz_q), None
        if not ok:
            order['status']='REFUNDED'; store.save(order)
            self._json(200,{'code':'REFUNDED','message':f'已退款:{err}',
                           'out_trade_no':oid}); return
        order['status']='FULFILLED'; order['result']=content; store.save(order)
        self._json(200,{'code':'SUCCESS','message':'付费内容',
                       'out_trade_no':oid,'content':content,
                       'already_fulfilled':False})

    def _handle_notify(self, data, new_status):
        oid = data.get('out_trade_no','') or data.get('out_trade_no','')
        if oid:
            o = store.get(oid)
            if o and o['status']=='INIT':
                o['status']=new_status; store.save(o)
        self._json(200,{"code":"SUCCESS"})

    def _json(self, s, d):
        self.send_response(s)
        self.send_header('Content-Type','application/json; charset=utf-8')
        self.end_headers()
        self.wfile.write(json.dumps(d,ensure_ascii=False).encode())

def _gen_content(q):
    return (f"【付费分析报告】\n查询:{q}\n时间:{datetime.now()}\n"
            f"━━━━━━━━━━━━━━━━━\n📊 关键发现\n"
            f"文档字数:约12,450字\n核心主题:3个\n\n"
            f"本次费用:¥0.50|微信AI专属卡支付")

def main():
    p = argparse.ArgumentParser()
    p.add_argument('--port',type=int,default=8080)
    p.add_argument('--production',action='store_true')
    args = p.parse_args()
    global MOCK_MODE
    MOCK_MODE = not args.production
    print(f"商户后端 | 模式:{'MOCK' if MOCK_MODE else 'PROD'} | :{args.port}")
    s = http.server.HTTPServer(('0.0.0.0',args.port), Handler)
    try:
        s.serve_forever()
    except KeyboardInterrupt:
        s.server_close()

if __name__ == '__main__':
    main()
