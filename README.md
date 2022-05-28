# gsv

## 这是一个简单的golang rpc框架，希望对服务定义，发布，注册，发现，调用，跟踪等域进行核心抽象，底层分别实现grpc或者简单的rest服务器。

### 分包
1、cli client客户端调用
2、discovery 服务注册以及发现
3、logger 日志
4、service 服务定义
5、server server承载多个服务
6、tracex 服务链路跟踪基础包
7、utils基础工具类

### 链路跟踪主要依赖open-telemetry

### 待续...
