## TiUp server


### Description
略微改动版的 tiup server，用于 QA 测试支持；原版 server 作为一个下游镜像，当有新的 component 发布到 server 时，会永久地与上游 fork

改动增加了一个 merge 的功能，tiup 系列操作普遍都会先获取 `timestamps.json` 和 `snapshot.json`，判断是否需要更新 manifest 文件，然后再继续后续操作<br>
于是在这里做了一个拦截，server 在接收到对 `timestamps.json` 的请求后，会先用 `singleflight` 调用 server 本地的 tiup list，询问上游是否有 manifest 更新，若有更新则先将更新 merge 到本地 manifest
然后再继续处理剩下的请求。

merge 时先把上游的 manifest 和本地的文件内容做合并，然后对合并后的 manifest 重签名，并替换原本的 manifest。因为要做签名，所以需要配置对应的 manifest private key，下面会描述如何部署

### 部署
#### 需求
1. tiup mirror genkey 生成的密钥对，分别有 root， snapshot， timestamp， index， 和 owner (很多)
2. 更新上游会调用 tiup list 对应的函数，所以 TIUP_HOME 对应的目录需要可用
3. 生成的一堆私钥，用来重签名

##### TODO：
详细描述/给个脚本

目前可以参考下 `Dockfile` 和 `tiupserver.yaml`，但是因为安全性问题，私钥那些已经手动配置了 k8s secret 了
具体部署步骤待完善