## TiUp server

### 描述
相比原版增加了一个 merge manifest 的功能，做法是：本地有一份上游 manifest 的 cache，当本地接受到 `GET timestamps.json` 的请求时，会从上游拉 manifest 跟 cache 比对，如果上游有更新则先将更新过的 manifest merge；后续处理逻辑不变

### 部署
1. `tiup mirror init <path>`
    初始化 mirror，会在 path 下生成 mirror 的公私钥
    生成内容如下：
    ```sh
    $ mkdir mirror && tiup mirror init mirror
    $ tree mirror 
    mirror
    ├── 1.index.json
    ├── keys
    │   ├── 098f6ce4340cc61f-root.json
    │   ├── 1a96fe3675843120-root.json
    │   ├── 22394b41e90f908c-timestamp.json
    │   ├── 60d6c7031c7165b9-index.json
    │   ├── 921a968250d03624-snapshot.json
    │   └── f5ec29cb314d8848-root.json
    ├── root.json
    ├── snapshot.json
    └── timestamp.json
    ```

2. `mkdir -p upstream/bin && mkdir -p upstream/manifests && curl -sL -o upstream/bin/root.json https://tiup-mirrors.pingcap.com/root.json`

    下载上游的 root.json 文件，用于从上游拉 manifest
    tiup 对目录结构有要求，所以 upstream 目录跟本地 $TIUPHOME 的结构是一样的

    ```sh
    upstream
    ├── bin
    │   └── root.json
    └── manifests
    ```


3. `tiup mirror genkey -n <keyname> --save`
    生成 component owner 的公私钥，用于在 publish 的时候签名

    其中私钥会保存在 `$TIUPHOME/keys` 下，公钥会保存在当前目录下，把这俩密钥移到 `mirror/private` 下，分别命名为 `public.json` 和 `private.json`

    这对密钥有两个地方会用到：

    一 是 server 启动时需要指定 owner key pair，server 会自动做 `tiup mirror grant` 操作，否则这对密钥无法用于 publish

    二 是 CI 的脚本在 publish 的时候要用到私钥，方便起见就把密钥放在能下载的目录下

4. run
   以上步骤完成后，目录结构应该如下：
    ```sh
    .
    ├── mirror # 提供镜像服务的目录，其中的文件均被 fileserver 暴露，可被直接访问
    │   ├── 1.index.json
    │   ├── keys # 镜像的私钥，考虑安全的情况下该放在 mirror 目录外
    │   │   ├── 098f6ce4340cc61f-root.json
    │   │   ├── 1a96fe3675843120-root.json
    │   │   ├── 22394b41e90f908c-timestamp.json
    │   │   ├── 60d6c7031c7165b9-index.json
    │   │   ├── 921a968250d03624-snapshot.json
    │   │   └── f5ec29cb314d8848-root.json
    │   ├── private # 便于 ci 脚本做 publish 操作，把 owner key 放这里方便直接下载
    │   │   ├── private.json
    │   │   └── public.json
    │   ├── root.json
    │   ├── snapshot.json
    │   └── timestamp.json
    └── upstream # 上游的 manifest cache 目录，用于 merge manifest
        ├── bin
        │   └── root.json
        └── manifests
    ```

    然后启动：
    ```sh
    $ ./tiup-server ./mirror \
     --index mirror/keys/60d6c7031c7165b9-index.json \
     --snapshot mirror/keys/921a968250d03624-snapshot.json \
     --timestamp mirror/keys/22394b41e90f908c-timestamp.json \
     --tiuphome upstream \
     --owner mirror/private/private.json \
     --ownerpub mirror/private/public.json
    ```

5. k8s 部署
  
    参考 [Dockerfile](Dockerfile) 和 [K8S Resources](tiupserver.yaml)

    引入对应文件即可


### 注意事项
1. **publish 时不要用跟上游一样的 version**，比如 publish `tidb v4.0.9` 这种，会有冲突导致这个版本不可用。 
    
    如果不小心 publish 了可以把 `mirror` 目录下对应的 tarball 删掉，然后重启 server 触发 manifest 更新解决。或者对于这个版本，临时切到上游镜像使用。

   原因是：在 merge manifest 时，base manifest 是上游，如果同样的组件版本在本地和上游均有 publish，则 manifest 以上游优先，但下载组件 tarball 时，又是以本地优先，导致 tarball 对不上 manifest 的签名，客户端会报错无法下载。 

2. 以上部署方式在客户端从上游镜像切到此镜像时，需要用 sever 的 `mirror/root.json` 替换客户端的 $TIUPHOME/bin/root.json，不然会报签名错误。
    
    避免方式：在部署时用上游的 mirror 公私钥 (文件名带 timestamp snapshot index root 的几个 json 文件) 替换 `tiup mirror init` 生成的公私钥，私钥需要寻找上游镜像的维护者索取，注意私钥安全
    (理论上使用后不需要切换镜像，因为下游是上游的超集)
   
    原因：tiup 在切换镜像源时，不会替换 root.json 文件，此文件相当于 CA，新镜像的 manifest 均由新 CA 签名，但客户端仍然使用原 CA 去验证合法性。替换 root.json 相当于换了 CA

    PS: mirror key 需要替换，但 owner key 不需要；因为 server 启动时会把传入的 owner key 写到 manifest 中，所以 owner key 直接用生成的即可。且 owner key 必须放在可下载的目录供 CI 使用，直接使用上游的 key 有安全隐患。

3. 有极低概率遇到 server 的 publish 操作卡住的情况，是逻辑 bug，可以重启 server 解决
   上游新版已经修复，但因为一些接口改动稍大，暂未跟进。