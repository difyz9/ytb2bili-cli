package com.example.weixinpayx402demo;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.wechat.pay.java.core.Config;
import com.wechat.pay.java.core.RSAAutoCertificateConfig;
import com.wechat.pay.java.core.notification.NotificationConfig;
import com.wechat.pay.java.core.notification.NotificationParser;
import com.wechat.pay.java.core.notification.RequestParam;
import com.wechat.pay.java.service.payments.nativepay.NativePayService;
import com.wechat.pay.java.service.payments.nativepay.model.Amount;
import com.wechat.pay.java.service.payments.nativepay.model.PrepayRequest;
import com.wechat.pay.java.service.payments.nativepay.model.PrepayResponse;
import com.wechat.pay.java.service.payments.nativepay.model.QueryOrderByOutTradeNoRequest;
import com.wechat.pay.java.service.payments.model.Transaction;
import com.wechat.pay.java.service.refund.RefundService;
import com.wechat.pay.java.service.refund.model.AmountReq;
import com.wechat.pay.java.service.refund.model.CreateRequest;
import com.wechat.pay.java.service.refund.model.Refund;
import com.wechat.pay.java.service.refund.model.RefundNotification;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.http.*;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.client.RestTemplate;
import jakarta.servlet.http.HttpServletRequest;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.Signature;
import java.security.spec.PKCS8EncodedKeySpec;
import java.time.LocalDateTime;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.Base64;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.stream.Collectors;

/**
 * 微信 Agent Pay 商户接入 Demo - Java 版
 *
 * 本 Demo 演示商户如何接入微信 Agent Pay（X402 协议），实现 Agent 付费资源获取：
 * 1. Agent 首次请求 → 商户调用微信 Native 下单 + AI 预下单 → 返回 HTTP 402
 * 2. Agent 支付成功后通过 Header X-Out-Trade-No 携带订单号重试 → 商户调微信查单验证 → 返回付费内容
 *
 * 核心要点：
 * - 微信支付 Native 下单 / 查单 / 退款：使用微信支付官方 SDK（wechatpay-java）
 * - AI 预下单：使用 SkillHub 颁发的开发者密钥签名（SKILLHUB-SHA256-RSA2048）
 *
 * 环境变量:
 *   # 微信支付商户配置（用于 Native 下单 + 查单 + 退款）
 *   MCH_ID            - 微信支付商户号
 *   APP_ID            - 应用ID
 *   SERIAL_NO         - 商户证书序列号
 *   PRIVATE_KEY_PATH  - 商户私钥文件路径（PEM 格式）
 *   MCH_APIV3_KEY     - 商户 APIv3 密钥（用于回调解密）
 *   PAY_NOTIFY_URL    - 支付结果通知地址
 *   REFUND_NOTIFY_URL - 退款结果通知地址
 *
 *   # SkillHub 开发者密钥配置（用于 AI 预下单签名）
 *   SKILLHUB_DEVELOPER_ID  - SkillHub 商户号，格式: sh-XXXXXXXX
 *   SKILLHUB_PUB_KEY_ID    - 密钥 ID，格式: PUB_KEY_xxx
 *   SKILLHUB_PRIVATE_KEY   - SkillHub 颁发的 RSA 2048 私钥（PEM 格式）
 *
 *   # Skill 信息
 *   SKILL_ID        - 在 SkillHub 发布的 Skill slug
 *   SKILL_VERSION   - Skill 版本号
 */
@SpringBootApplication
@RestController
@RequestMapping("/api")
public class WeixinPayX402DemoApplication {

    // ==================== 微信支付商户配置 ====================

    private final String mchId = getEnv("MCH_ID", "");
    private final String appId = getEnv("APP_ID", "");
    private final String serialNo = getEnv("SERIAL_NO", "");
    private final String privateKeyPath = getEnv("PRIVATE_KEY_PATH", "");  // 商户私钥文件路径
    private final String mchAPIv3Key = getEnv("MCH_APIV3_KEY", "");       // APIv3 密钥
    private final String payNotifyUrl = getEnv("PAY_NOTIFY_URL", "https://example.com/api/pay/notify");       // 支付回调地址
    private final String refundNotifyUrl = getEnv("REFUND_NOTIFY_URL", "https://example.com/api/refund/notify"); // 退款回调地址
    private static final int TIME_EXPIRE_SEC = 870; // 订单过期时间（秒），默认 14.5 分钟

    // ==================== SkillHub 开发者密钥配置 ====================

    private final String skillhubDeveloperId = getEnv("SKILLHUB_DEVELOPER_ID", "");
    private final String skillhubPubKeyId = getEnv("SKILLHUB_PUB_KEY_ID", "");
    private final String skillhubPrivateKey = getEnv("SKILLHUB_PRIVATE_KEY", "");

    // ==================== Skill 信息 ====================

    private final String skillId = getEnv("SKILL_ID", "");
    private final String skillVersion = getEnv("SKILL_VERSION", "1.0.0");

    // ==================== API 地址 ====================

    // AI 预下单接口（使用 SkillHub 开发者密钥签名）
    private static final String AI_PREORDER_URL = "https://payapp.weixin.qq.com/palmpayminiapp/clawagentpay/preorder";

    // ==================== 微信支付 SDK 客户端 ====================

    private NativePayService nativePayService;
    private RefundService refundService;
    private NotificationParser notificationParser; // SDK 回调验签+解密处理器

    // ==================== 订单存储（内存版，生产环境请用数据库）====================

    private final ConcurrentHashMap<String, OrderRecord> orderStore = new ConcurrentHashMap<>();
    private final ObjectMapper objectMapper = new ObjectMapper();
    private final RestTemplate restTemplate = new RestTemplate();

    // ==================== 数据结构 ====================

    static class OrderRecord {
        public String outTradeNo;
        public String paymentCode;
        public int amount;
        public String description;
        public String query;
        public String status; // INIT / PAID / FULFILLED / REFUNDED
        public String result;
        public String refundNo;
        public String refundReason;
        public String createdAt;
    }

    static class ResourceRequest {
        public String query;
    }

    // ==================== 主程序 ====================

    public static void main(String[] args) {
        SpringApplication.run(WeixinPayX402DemoApplication.class, args);
        System.out.println("微信 Agent Pay 商户 Demo 启动成功");
        System.out.println("接口地址: POST http://localhost:8080/api/resource");
    }

    /**
     * 初始化微信支付 SDK 客户端
     * 使用官方 SDK: com.github.wechatpay-apiv3:wechatpay-java
     */
    @jakarta.annotation.PostConstruct
    public void initWechatPayClient() {
        Config config = new RSAAutoCertificateConfig.Builder()
            .merchantId(mchId)
            .privateKeyFromPath(privateKeyPath)
            .merchantSerialNumber(serialNo)
            .apiV3Key(mchAPIv3Key)
            .build();

        nativePayService = new NativePayService.Builder().config(config).build();
        refundService = new RefundService.Builder().config(config).build();

        // 初始化回调通知解析器（验签 + AES-256-GCM 解密）
        notificationParser = new NotificationParser((NotificationConfig) config);
        System.out.println("微信支付 SDK 客户端初始化成功");
    }

    // ==================== 核心处理逻辑 ====================

    @PostMapping("/resource")
    public ResponseEntity<Map<String, Object>> handleResource(
            @RequestBody ResourceRequest req,
            @RequestHeader(value = "X-Out-Trade-No", required = false) String outTradeNo) {
        if (outTradeNo != null && !outTradeNo.isEmpty()) {
            return verifyAndFulfill(outTradeNo, req.query);
        }
        return createPaymentOrder(req);
    }

    @GetMapping("/health")
    public String health() {
        return "ok";
    }

    /**
     * 创建支付订单：Native 下单 → AI 预下单 → 返回 402
     */
    private ResponseEntity<Map<String, Object>> createPaymentOrder(ResourceRequest req) {
        try {
            // 1. 生成商户订单号（不超过32位）
            String outTradeNo = generateOutTradeNo();

            // 2. 调用微信支付 Native 下单（使用微信支付官方 SDK）
            int amount = 1; // 0.01 元 = 1 分
            String description = "AI付费查询: " + truncate(req.query, 50);

            String codeUrl = callNativeOrder(outTradeNo, description, amount);

            // 3. 调用 AI 预下单接口（使用 SkillHub 开发者密钥签名），获取 payment_code
            String paymentCode = callAIPreorder(codeUrl);

            // 4. 保存订单记录
            OrderRecord order = new OrderRecord();
            order.outTradeNo = outTradeNo;
            order.paymentCode = paymentCode;
            order.amount = amount;
            order.description = description;
            order.query = req.query;
            order.status = "INIT";
            order.createdAt = LocalDateTime.now().format(DateTimeFormatter.ISO_LOCAL_DATE_TIME);
            orderStore.put(outTradeNo, order);

            System.out.printf("订单创建成功: out_trade_no=%s, payment_code=%s%n", outTradeNo, paymentCode);

            // 5. 返回 HTTP 402 + WeixinPay-Required + X-Out-Trade-No
            HttpHeaders headers = new HttpHeaders();
            headers.setContentType(MediaType.APPLICATION_JSON);
            headers.set("WeixinPay-Required", paymentCode);
            headers.set("X-Out-Trade-No", outTradeNo);

            Map<String, Object> body = Map.of(
                "code", "PAYMENT_REQUIRED",
                "message", "需要支付后才能获取内容",
                "WeixinPay", Map.of(
                    "WeixinPay-Required", paymentCode,
                    "prompt", "本次使用微信支付，请将 WeixinPay-Required 的值作为 paymentCode 交给 weixinpay_pay，以向用户申请支付授权。"
                ),
                "out_trade_no", outTradeNo,
                "amount", String.format("%.2f", amount / 100.0),
                "currency", "CNY",
                "description", description
            );

            return new ResponseEntity<>(body, headers, HttpStatus.PAYMENT_REQUIRED);

        } catch (Exception e) {
            System.err.println("创建订单失败: " + e.getMessage());
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                .body(Map.of("error", "创建支付订单失败: " + e.getMessage()));
        }
    }

    /**
     * 验证支付并履约：查单验证 → 返回付费内容
     * outTradeNo 从请求 Header X-Out-Trade-No 中获取
     */
    private ResponseEntity<Map<String, Object>> verifyAndFulfill(String outTradeNo, String query) {

        // 1. 查询本地订单记录
        OrderRecord order = orderStore.get(outTradeNo);
        if (order == null) {
            return ResponseEntity.status(HttpStatus.NOT_FOUND)
                .body(Map.of("error", "订单不存在: " + outTradeNo));
        }

        // 2. 幂等：如果已经履约过，直接返回缓存结果
        if ("FULFILLED".equals(order.status)) {
            System.out.printf("订单已履约，返回缓存结果: out_trade_no=%s%n", outTradeNo);
            return ResponseEntity.ok(Map.of(
                "code", "SUCCESS",
                "message", "付费内容（已缓存）",
                "out_trade_no", outTradeNo,
                "content", order.result,
                "already_fulfilled", true
            ));
        }

        try {
            // 3. 调用微信支付查单 API 验证支付状态（使用微信支付官方 SDK）
            Transaction transaction = callQueryOrder(outTradeNo);

            // 4. 校验支付状态
            String tradeState = transaction.getTradeState().name();
            if (!"SUCCESS".equals(tradeState)) {
                System.out.printf("订单未支付: out_trade_no=%s, trade_state=%s%n", outTradeNo, tradeState);
                return ResponseEntity.status(HttpStatus.PAYMENT_REQUIRED)
                    .body(Map.of(
                        "code", "NOT_PAID",
                        "message", "订单尚未支付完成",
                        "out_trade_no", outTradeNo,
                        "trade_state", tradeState
                    ));
            }

            // 5. 支付成功，执行业务逻辑（生成付费内容）
            String bizQuery = (query != null && !query.isEmpty()) ? query : order.query;
            String result;
            try {
                result = executeBusiness(bizQuery);
            } catch (RuntimeException bizErr) {
                // 业务逻辑执行失败，主动发起退款
                System.err.printf("业务执行失败，发起退款: out_trade_no=%s, error=%s%n", outTradeNo, bizErr.getMessage());
                try {
                    callRefund(outTradeNo, order.amount, "服务异常无法提供内容: " + bizErr.getMessage());
                    order.status = "REFUNDED";
                    order.refundReason = bizErr.getMessage();
                    return ResponseEntity.ok(Map.of(
                        "code", "REFUNDED",
                        "message", "服务无法提供付费内容，已发起全额退款",
                        "out_trade_no", outTradeNo,
                        "refund_reason", bizErr.getMessage()
                    ));
                } catch (Exception refundErr) {
                    System.err.printf("退款请求失败: out_trade_no=%s, error=%s%n", outTradeNo, refundErr.getMessage());
                    return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                        .body(Map.of(
                            "code", "FULFILL_AND_REFUND_FAILED",
                            "message", "服务异常且退款失败，请联系客服",
                            "out_trade_no", outTradeNo
                        ));
                }
            }

            // 6. 更新订单状态为已履约
            order.status = "FULFILLED";
            order.result = result;

            String transactionId = transaction.getTransactionId() != null
                ? transaction.getTransactionId() : "";
            System.out.printf("履约成功: out_trade_no=%s, transaction_id=%s%n", outTradeNo, transactionId);

            // 7. 返回付费内容
            return ResponseEntity.ok(Map.of(
                "code", "SUCCESS",
                "message", "付费内容",
                "out_trade_no", outTradeNo,
                "transaction_id", transactionId,
                "content", result,
                "already_fulfilled", false
            ));

        } catch (Exception e) {
            // 查单失败返回 402，让 Agent 稍后重试（可能是网络抖动，不一定是服务端错误）
            System.err.println("查单/履约失败: " + e.getMessage());
            return ResponseEntity.status(HttpStatus.PAYMENT_REQUIRED)
                .body(Map.of(
                    "code", "NOT_PAID",
                    "message", "支付状态校验中，请稍后重试",
                    "out_trade_no", outTradeNo
                ));
        }
    }

    // ==================== 业务逻辑 ====================

    /**
     * 执行业务逻辑（模拟）
     * 生产环境中，这里是真正的业务处理，如调用下游 API、生成报告等
     * 抛出 RuntimeException 表示不可恢复的失败，将触发退款
     */
    private String executeBusiness(String query) {
        // 模拟：如需测试退款流程，可将 query 设为 "trigger_refund" 触发模拟失败
        if ("trigger_refund".equals(query)) {
            throw new RuntimeException("下游服务不可用，无法生成内容");
        }
        return generatePaidContent(query);
    }

    // ==================== 微信支付 Native 下单 & 查单 & 退款（官方 SDK）====================

    /**
     * 调用微信支付 Native 下单接口
     * 使用微信支付官方 SDK: com.github.wechatpay-apiv3:wechatpay-java
     * 文档: https://pay.weixin.qq.com/doc/v3/merchant/4012791877
     */
    private String callNativeOrder(String outTradeNo, String description, int amount) throws Exception {
        PrepayRequest request = new PrepayRequest();
        request.setAppid(appId);
        request.setMchid(mchId);
        request.setDescription(description);
        request.setOutTradeNo(outTradeNo);
        request.setNotifyUrl(payNotifyUrl);

        // 设置订单过期时间，避免订单永不过期
        OffsetDateTime timeExpire = OffsetDateTime.now(ZoneOffset.of("+08:00"))
            .plusSeconds(TIME_EXPIRE_SEC);
        request.setTimeExpire(timeExpire.format(DateTimeFormatter.ISO_OFFSET_DATE_TIME));

        Amount amountObj = new Amount();
        amountObj.setTotal(amount);
        amountObj.setCurrency("CNY");
        request.setAmount(amountObj);

        PrepayResponse response = nativePayService.prepay(request);
        return response.getCodeUrl();
    }

    /**
     * 调用微信支付查单接口
     * 使用微信支付官方 SDK
     * 文档: https://pay.weixin.qq.com/doc/v3/merchant/4012791880
     */
    private Transaction callQueryOrder(String outTradeNo) throws Exception {
        QueryOrderByOutTradeNoRequest request = new QueryOrderByOutTradeNoRequest();
        request.setOutTradeNo(outTradeNo);
        request.setMchid(mchId);
        return nativePayService.queryOrderByOutTradeNo(request);
    }

    /**
     * 调用微信支付退款接口
     * 使用微信支付官方 SDK
     * 文档: https://pay.weixin.qq.com/doc/v3/merchant/4012791883
     */
    private void callRefund(String outTradeNo, int amount, String reason) throws Exception {
        String outRefundNo = "RF_" + outTradeNo.substring(6);

        CreateRequest request = new CreateRequest();
        request.setOutTradeNo(outTradeNo);
        request.setOutRefundNo(outRefundNo);
        request.setReason(reason.length() > 80 ? reason.substring(0, 80) : reason);
        request.setNotifyUrl(refundNotifyUrl);

        AmountReq amountReq = new AmountReq();
        amountReq.setRefund(Long.valueOf(amount));
        amountReq.setTotal(Long.valueOf(amount));
        amountReq.setCurrency("CNY");
        request.setAmount(amountReq);

        Refund refund = refundService.create(request);
        System.out.printf("退款受理成功: out_trade_no=%s, out_refund_no=%s, status=%s%n",
            outTradeNo, outRefundNo, refund.getStatus());

        // 更新订单退款单号
        OrderRecord order = orderStore.get(outTradeNo);
        if (order != null) {
            order.refundNo = outRefundNo;
        }
    }

    /**
     * 微信支付结果回调通知处理
     * 使用 SDK NotificationParser：自动验证签名 + AES-256-GCM 解密通知报文
     * 文档: https://pay.weixin.qq.com/doc/v3/merchant/4012791877
     */
    @PostMapping("/pay/notify")
    public ResponseEntity<Map<String, Object>> handlePayNotify(HttpServletRequest request) {
        try {
            // 从 HTTP 请求中提取回调参数
            RequestParam requestParam = buildRequestParam(request);

            // 使用 SDK 验签 + 解密
            Transaction transaction = notificationParser.parse(requestParam, Transaction.class);

            String outTradeNo = transaction.getOutTradeNo();
            String tradeState = transaction.getTradeState().name();
            String transactionId = transaction.getTransactionId();
            System.out.printf("[pay_notify] out_trade_no=%s trade_state=%s transaction_id=%s%n",
                outTradeNo, tradeState, transactionId);

            // 仅当支付成功时更新本地订单状态
            if ("SUCCESS".equals(tradeState) && outTradeNo != null) {
                OrderRecord order = orderStore.get(outTradeNo);
                if (order != null && "INIT".equals(order.status)) {
                    order.status = "PAID";
                    System.out.printf("[pay_notify] 订单状态更新: out_trade_no=%s status=PAID%n", outTradeNo);
                }
            }

            return ResponseEntity.ok(Map.of("code", "SUCCESS", "message", "成功"));
        } catch (Exception e) {
            System.err.println("[pay_notify] 验签/解密失败: " + e.getMessage());
            return ResponseEntity.status(HttpStatus.BAD_REQUEST)
                .body(Map.of("code", "FAIL", "message", e.getMessage()));
        }
    }

    /**
     * 微信支付退款结果回调通知处理
     * 使用 SDK NotificationParser：自动验证签名 + AES-256-GCM 解密通知报文
     * 文档: https://pay.weixin.qq.com/doc/v3/merchant/4012791888
     */
    @PostMapping("/refund/notify")
    public ResponseEntity<Map<String, Object>> handleRefundNotify(HttpServletRequest request) {
        try {
            // 从 HTTP 请求中提取回调参数
            RequestParam requestParam = buildRequestParam(request);

            // 使用 SDK 验签 + 解密
            RefundNotification refundNotification = notificationParser.parse(requestParam, RefundNotification.class);

            String outTradeNo = refundNotification.getOutTradeNo();
            String outRefundNo = refundNotification.getOutRefundNo();
            String refundStatus = refundNotification.getRefundStatus().name();
            System.out.printf("[refund_notify] out_trade_no=%s out_refund_no=%s status=%s%n",
                outTradeNo, outRefundNo, refundStatus);

            // 根据退款状态更新本地订单
            if (outTradeNo != null) {
                OrderRecord order = orderStore.get(outTradeNo);
                if (order != null) {
                    order.refundNo = outRefundNo;
                    switch (refundStatus) {
                        case "SUCCESS":
                            order.status = "REFUNDED";
                            System.out.printf("[refund_notify] 退款成功: out_trade_no=%s%n", outTradeNo);
                            break;
                        case "CLOSED":
                            System.out.printf("[refund_notify] 退款关闭: out_trade_no=%s%n", outTradeNo);
                            break;
                        case "ABNORMAL":
                            System.out.printf("[refund_notify] 退款异常（需人工介入）: out_trade_no=%s%n", outTradeNo);
                            break;
                    }
                }
            }

            return ResponseEntity.ok(Map.of("code", "SUCCESS", "message", "成功"));
        } catch (Exception e) {
            System.err.println("[refund_notify] 验签/解密失败: " + e.getMessage());
            return ResponseEntity.status(HttpStatus.BAD_REQUEST)
                .body(Map.of("code", "FAIL", "message", e.getMessage()));
        }
    }

    /**
     * 从 HttpServletRequest 中构建 SDK 所需的 RequestParam
     */
    private RequestParam buildRequestParam(HttpServletRequest request) throws Exception {
        String body = new BufferedReader(new InputStreamReader(request.getInputStream(), StandardCharsets.UTF_8))
            .lines().collect(Collectors.joining("\n"));
        String timestamp = request.getHeader("Wechatpay-Timestamp");
        String nonce = request.getHeader("Wechatpay-Nonce");
        String signature = request.getHeader("Wechatpay-Signature");
        String serial = request.getHeader("Wechatpay-Serial");
        String signType = request.getHeader("Wechatpay-Signature-Type");
        if (signType == null || signType.isEmpty()) {
            signType = "WECHATPAY2-SHA256-RSA2048";
        }

        return new RequestParam.Builder()
            .serialNumber(serial)
            .nonce(nonce)
            .signature(signature)
            .timestamp(timestamp)
            .signType(signType)
            .body(body)
            .build();
    }

    // ==================== AI 预下单（SkillHub 开发者密钥签名）====================

    /**
     * 调用 AI 预下单接口
     * 注意：此接口使用 SkillHub 颁发的开发者密钥签名，而非微信支付 API 证书
     *
     * 流程：
     * 1. 构造 L2 业务 JSON
     * 2. Base64 编码 L2
     * 3. 构造签名串（5 行）
     * 4. SHA256withRSA 签名（使用 SkillHub 私钥）
     * 5. 组装 L1 请求体并发送
     */
    private String callAIPreorder(String codeUrl) throws Exception {
        // 1. 构造 L2 业务 JSON
        ObjectNode l2 = objectMapper.createObjectNode();

        ObjectNode skillInfo = objectMapper.createObjectNode();
        skillInfo.put("skill_id", skillId);
        skillInfo.put("skill_version", skillVersion);
        l2.set("skill_info", skillInfo);

        l2.put("pay_type", "SKILL_PAY");
        l2.put("pay_mode", "AUTH_AND_PAY");

        ArrayNode payItems = objectMapper.createArrayNode();
        ObjectNode payItem = objectMapper.createObjectNode();
        payItem.put("product_id", "SP" + LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyMMdd")) + "A001");
        ObjectNode payData = objectMapper.createObjectNode();
        payData.put("type", "code_url");
        payData.put("value", codeUrl);
        payItem.set("pay_data", payData);
        payItems.add(payItem);
        l2.set("pay_items", payItems);

        l2.put("expires_at", String.valueOf(System.currentTimeMillis() / 1000 + 15 * 60));

        String l2Json = objectMapper.writeValueAsString(l2);

        // 2. Base64 编码 L2
        String paymentRequired = Base64.getEncoder().encodeToString(l2Json.getBytes(StandardCharsets.UTF_8));

        // 3. 构造签名串（5 行，每行以 \n 结尾）
        String timestamp = String.valueOf(System.currentTimeMillis() / 1000);
        String nonceStr = UUID.randomUUID().toString().replace("-", "");
        String signString = "POST\n/palmpayminiapp/clawagentpay/preorder\n"
            + timestamp + "\n" + nonceStr + "\n" + paymentRequired + "\n";

        // 4. SHA256withRSA 签名（使用 SkillHub 开发者私钥）
        String signature = signWithSkillHubKey(signString);

        // 5. 组装 L1 请求体
        ObjectNode l1 = objectMapper.createObjectNode();
        l1.put("signature_type", "SKILLHUB-SHA256-RSA2048");
        l1.put("developer_platform", "SKILLHUB");
        l1.put("developer_id", skillhubDeveloperId);
        l1.put("pub_key_id", skillhubPubKeyId);
        l1.put("nonce_str", nonceStr);
        l1.put("timestamp", timestamp);
        l1.put("signature", signature);
        l1.put("payment_required", paymentRequired);

        // 6. 发送请求
        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);

        HttpEntity<String> entity = new HttpEntity<>(objectMapper.writeValueAsString(l1), headers);
        ResponseEntity<String> response = restTemplate.exchange(
            AI_PREORDER_URL, HttpMethod.POST, entity, String.class);

        if (!response.getStatusCode().is2xxSuccessful()) {
            throw new RuntimeException(String.format("AI预下单失败, HTTP %d: %s",
                response.getStatusCode().value(), response.getBody()));
        }

        JsonNode result = objectMapper.readTree(response.getBody());
        if (!result.has("payment_code")) {
            throw new RuntimeException("AI预下单响应中缺少 payment_code: " + response.getBody());
        }

        return result.get("payment_code").asText();
    }

    /**
     * 使用 SkillHub 开发者私钥进行 SHA256withRSA 签名
     *
     * 签名算法: SKILLHUB-SHA256-RSA2048
     * - 密钥算法: RSA 2048
     * - 哈希算法: SHA-256
     * - 签名方式: PKCS#1 v1.5
     * - 签名编码: 标准 Base64（非 URL-safe）
     */
    private String signWithSkillHubKey(String signString) throws Exception {
        // 解析 PEM 格式私钥（支持 PKCS#8 和 PKCS#1 格式）
        String pemContent = skillhubPrivateKey
            .replace("-----BEGIN PRIVATE KEY-----", "")
            .replace("-----END PRIVATE KEY-----", "")
            .replace("-----BEGIN RSA PRIVATE KEY-----", "")
            .replace("-----END RSA PRIVATE KEY-----", "")
            .replaceAll("\\s+", "");

        byte[] keyBytes = Base64.getDecoder().decode(pemContent);

        PrivateKey key;
        try {
            // 优先尝试 PKCS#8 格式
            PKCS8EncodedKeySpec keySpec = new PKCS8EncodedKeySpec(keyBytes);
            KeyFactory keyFactory = KeyFactory.getInstance("RSA");
            key = keyFactory.generatePrivate(keySpec);
        } catch (Exception e) {
            // 回退尝试 PKCS#1 格式（需要转换为 PKCS#8）
            // PKCS#1 格式的 RSA 私钥需要包装为 PKCS#8
            byte[] pkcs8Bytes = convertPKCS1ToPKCS8(keyBytes);
            PKCS8EncodedKeySpec pkcs8Spec = new PKCS8EncodedKeySpec(pkcs8Bytes);
            KeyFactory keyFactory = KeyFactory.getInstance("RSA");
            key = keyFactory.generatePrivate(pkcs8Spec);
        }

        // SHA256withRSA 签名
        Signature sig = Signature.getInstance("SHA256withRSA");
        sig.initSign(key);
        sig.update(signString.getBytes(StandardCharsets.UTF_8));

        // 标准 Base64 编码
        return Base64.getEncoder().encodeToString(sig.sign());
    }

    /**
     * 将 PKCS#1 格式的 RSA 私钥转换为 PKCS#8 格式
     */
    private byte[] convertPKCS1ToPKCS8(byte[] pkcs1Bytes) {
        // PKCS#8 header for RSA
        byte[] pkcs8Header = {
            0x30, (byte) 0x82, 0x00, 0x00, // SEQUENCE (length placeholder)
            0x02, 0x01, 0x00,              // INTEGER 0 (version)
            0x30, 0x0d,                    // SEQUENCE
            0x06, 0x09,                    // OID
            0x2a, (byte) 0x86, 0x48, (byte) 0x86, (byte) 0xf7, 0x0d, 0x01, 0x01, 0x01, // rsaEncryption
            0x05, 0x00,                    // NULL
            0x04, (byte) 0x82, 0x00, 0x00  // OCTET STRING (length placeholder)
        };

        int totalLength = pkcs8Header.length + pkcs1Bytes.length;
        byte[] pkcs8Bytes = new byte[totalLength];
        System.arraycopy(pkcs8Header, 0, pkcs8Bytes, 0, pkcs8Header.length);
        System.arraycopy(pkcs1Bytes, 0, pkcs8Bytes, pkcs8Header.length, pkcs1Bytes.length);

        // 修正长度字段
        int seqLength = totalLength - 4;
        pkcs8Bytes[2] = (byte) ((seqLength >> 8) & 0xff);
        pkcs8Bytes[3] = (byte) (seqLength & 0xff);

        int octetLength = pkcs1Bytes.length;
        pkcs8Bytes[pkcs8Header.length - 2] = (byte) ((octetLength >> 8) & 0xff);
        pkcs8Bytes[pkcs8Header.length - 1] = (byte) (octetLength & 0xff);

        return pkcs8Bytes;
    }

    // ==================== 工具方法 ====================

    /**
     * 生成商户订单号（不超过32位）
     * 格式: WX402_ + 时间戳(14位) + 随机串(12位) = 32位
     */
    private String generateOutTradeNo() {
        String ts = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyyMMddHHmmss"));
        String rand = UUID.randomUUID().toString().replace("-", "").substring(0, 12);
        return "WX402_" + ts + rand;
    }

    /**
     * 生成付费内容（模拟业务逻辑）
     */
    private String generatePaidContent(String query) {
        String now = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss"));
        return String.format(
            "【付费内容】针对您的查询「%s」，以下是详细分析结果：\n\n" +
            "1. 这是通过微信 Agent Pay 支付后获取的付费内容\n" +
            "2. 商户已通过微信查单 API 验证支付成功\n" +
            "3. 本内容具有幂等性，重复请求将返回相同结果\n\n" +
            "生成时间: %s",
            query, now
        );
    }

    private String truncate(String s, int maxLen) {
        if (s == null) return "";
        if (s.length() <= maxLen) return s;
        return s.substring(0, maxLen) + "...";
    }

    private static String getEnv(String key, String defaultValue) {
        String value = System.getenv(key);
        return value != null ? value : defaultValue;
    }
}
