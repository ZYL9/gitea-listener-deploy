# Intro
## 需求
项目push到gitea后，自动进行docker build及docker run

## 来由
需求分析一下其实就是究极简化版的cicd，网上搜了下，铺天盖地的Jenkins教程
但是Jenkins实在太耗内存，便宜虚机性能不行跑起来巨卡无比，查到的同类方案基本都是类似情况。
原本计划使用gitea action，由于网络的原因，本机部署的测试环境github action上的项目基本都拉不下来
加上action本身就是容器起的，action运行的时候就是容器套容器，操作起来非常的晦涩，有几个点满足不了要求，研究了一下午无果
加上本身需求比较简单，死磕action实在不划算
于是研究了一下webhook，起一个http server，捕获gitea传的webhook后，用脚本操作docker就行
## 项目地址
还没建，有空再说吧

# tldr
使用以下命令启动
```bash
docker compose up -d
```
启动后找一下`./gitea/gitea/conf/app.ini`
拉到最下面加上这个
```ini
[webhook]
ALLOWED_HOST_LIST = *
```

新建项目后，项目的设置界面找一下`Web钩子`
添加一个`gitea`的钩子
目标url：`http://listener:3001/webhook`
其他都不用动，可以点测试推送试一下能不能用

# 实现
## 技术选型
1. 尽量小体积
2. 不用解释语言
3. 不依赖第三方库

首先排除python(划掉)，不造轮子的话，最简单的http server都需要第三方库支撑，太复杂了
Java和c#都是太大太笨了，还需要jre或者runtime
rust和c++其实是优选，但是可惜都不太会

思考一下最后选了golang，可以不调用任何三方库，原生编译，也挺简单的
工具链也比较齐全，还能交叉编译

## 实现
把需求丢到gpt里，就能拿到模板了，主要就几个部分
下面贴上关键代码，尽量省略错误处理
golang的错误处理实在是有点难受

1. http server捕获post请求
```go
http.HandleFunc("/webhook", webhookHandler)
if err := http.ListenAndServe(":3001", nil); err != nil {
	log.Fatal(err)
}
```
2. 把post body里的json解析到字段
```go
body, err := io.ReadAll(r.Body)
defer r.Body.Close()
jsonBody := string(body)
//使用interface{}，不使用对象实现全量字段映射
var data map[string]interface{}
//解析JSON字符串
err = json.Unmarshal([]byte(jsonBody), &data)

//用map[string]interface{}匹配json下的对象
//Repo类
repository, ok := data["repository"].(map[string]interface{})
//项目名
repoName, ok := repository["name"].(string)
//用户名/项目名
fullName, ok := repository["full_name"].(string)
//地址
cloneUrl, ok := repository["clone_url"].(string)
```
3. 把项目拉下来，去重
```go
basePath := "/data/"
repoPath := basePath + repoName
//If the repository already exists, delete it first
if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
	err = os.RemoveAll(repoPath)
	log.Println("Delete exist folder")
	if err != nil {
		http.Error(w, "Error deleting existing repository: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

cmd := exec.Command("git", "clone", urlConverter(cloneUrl), repoPath)
output, err := cmd.CombinedOutput()
```
测试环境下cloneUrl的地址是localhost:3000，会报错，转换成docker虚拟网卡的地址比较稳
```go
// 把cloneUrl转为docker内地址
func urlConverter(url string) string {
	baseUrl := "http://gitea:3000/"
	path := strings.Split(url, "//")[1]
	// 使用/作为分隔符分割路径
	parts := strings.Split(path, "/")
	// 截取并拼接字符串
	result := baseUrl + strings.Join(parts[1:], "/")
	log.Println("localCloneUrl: " + result)
	return result
}
```
5. 从解析到的字段传到脚本
这里执行脚本的func起了一个新进程。
主要是因为gitea那边限制了回包时间，想要看到回显，以及不想看到记录那边一片红
```go
//如果git clone没报错才执行脚本
if err != nil {
	http.Error(w, "Error cloning repository: "+err.Error()+"\n"+string(output), http.StatusInternalServerError)
	return
} else {
	w.WriteHeader(http.StatusOK)
	w.Write(output)
	go dockerBuildAndRun(w, repoName, fullName, repoPath)
	return
}
```
```go
func dockerBuildAndRun(w http.ResponseWriter,
	repoName string, fullName string, repoPath string) {
	dockerCmd := exec.Command("sh", "/opt/start.sh", repoName, fullName, repoPath)
	dockerCmdOutput, err := dockerCmd.CombinedOutput()
	if err != nil {
		errorHandler(w, string(dockerCmdOutput))
		return
	}
	log.Println(string(dockerCmdOutput))
}
```
6. 脚本执行
先把同名的容器干掉，然后删掉同名的镜像，重新build并run
```bash
#!/bin/sh
# 1-repoName, 2-fullName, 3-repoPath
dn=$(docker ps -aq --filter name=${1})
echo " "
echo "Docker run and build start!"
echo "---------------------"
echo "docker stop $1"
echo "docker rm ${dn}"
echo "docker rmi "$2""
echo "docker build -t "$2" "$3""
echo "docker run -d --name "$1" -p 40030:80 "$2""

docker stop "$1"
docker rm ${dn}
docker rmi "$2"
docker build -t "$2" "$3"
docker run -d --name "$1" -p 40030:80 "$2"
```
顺便补充下go.mod
因为用的全部都是原生库，其实贴不贴没啥影响
```
module auto-deploy/listener

go 1.22.1
```
### Dockerfile&&Docker Compose

```Dockerfile
FROM golang:alpine3.19 as BUILD
ENV GOOS=linux
ENV GOARCH=amd64
WORKDIR /usr/src/app
COPY main.go go.mod ./
RUN go build .

FROM alpinelinux/docker-cli:latest as PROD
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apk/repositories &&\
    apk update &&\
    apk add git
COPY --from=BUILD /usr/src/app/listener /opt/
COPY start.sh /opt/
RUN chmod +x /opt/listener &&\
    chmod +x /opt/start.sh
EXPOSE 3001
CMD /opt/listener
```
起gitea的docker-compose.yaml
```yaml
version: "3.9"
services:
  gitea:
    image: gitea/gitea:1.21.7
    container_name: gitea
    environment:
      - USER_UID=1000
      - USER_GID=1000
    restart: always
    networks:
      gitea_network:
        aliases:
          - gitea
    volumes:
      - ./gitea:/data
      - /etc/timezone:/etc/timezone:ro
      - /etc/localtime:/etc/localtime:ro
      # - /home/xxx/.ssh/:/data/git/.ssh
    ports:
      - "3000:3000"
    expose:
      - 3000

  listener:
    image: zy/listener
    build: ./listener-alpine
    container_name: listener
    restart: always
    networks:
      gitea_network:
        aliases:
          - listener
    volumes:
      - ./listener-data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    expose:
      - 3001

networks:
  gitea_network:
    name: gitea_network
    driver: bridge
    external: false

```

再补一个前端项目的通用Dockerfile，不一定很通用，但是满足现在我的需求了
```Dockerfile
# ---- Base Node ----
FROM node:lts-alpine3.19 AS base
# 创建 app 目录
WORKDIR /app

# ---- Dependencies ----
FROM base AS dependencies  
# 使用通配符复制 package.json 与 package-lock.json
COPY package*.json ./
# 安装在‘devDependencies’中包含的依赖
#公网用阿里的npm源
#RUN npm install -g pnpm --registry=https://registry.npmmirror.com
#RUN pnpm install --registry=https://registry.npmmirror.com
#腾讯云上用内网腾讯npm源
RUN npm install -g pnpm --registry=http://mirrors.tencentyun.com/npm/
RUN pnpm install --registry=http://mirrors.tencentyun.com/npm/

# ---- Copy Files/Build ----
FROM dependencies AS build  
WORKDIR /app
COPY ./ /app
RUN pnpm run build

FROM nginx:stable-alpine-slim as prod
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

### todo
现在自动起的端口都是写死的`40030:80`，不太能满足后续变化，应该在程序里加一个判断，根据`repo-description`或者其他字段里的内容对端口映射进行调整。
但是现在这个主要是我自己的前端页面需要自动更新，暂时没有需求，就犯懒了。

## 坑
### 容器内操作docker失效
用普通alpine挂载sock后，映射/usr/bin/docker不行
还是需要装一个docker-cli
最简单的话，用`alpinelinux/docker-cli`做底，加一个git就好

### wsl出现各种诡异问题
包括但不限于
- docker desktop闪退
	- 重启wsl
- `docker compose Error response from daemon: network gitea_network not found`
	- 从docker compose创建网络失败，手动先建好网络，再起docker compose就好了

涉及docker的操作尽量不要用wsl，很折磨

### gitea报错
`Delivery: Post "http://listener:3001/webhook": dial tcp 172.18.0.3:3001: webhook can only call allowed HTTP servers (check your webhook.ALLOWED_HOST_LIST setting), deny 'listener(172.18.0.3:3001)'`

gitea默认的webhook不允许对环回及内网段发，需要额外加一个配置
位置在`gitea/conf/app.ini`
```ini
[webhook]
ALLOWED_HOST_LIST = *
```
在webhook里面没有写，翻全量配置里找到的
``` txt
Webhook can only call allowed hosts for security reasons. Comma separated list, eg: external, 192.168.1.0/24, *.mydomain.com
Built-in: loopback (for localhost), private (for LAN/intranet), external (for public hosts on internet), * (for all hosts)
CIDR list: 1.2.3.0/8, 2001:db8::/32
Wildcard hosts: *.mydomain.com, 192.168.100.*
Since 1.15.7. Default to * for 1.15.x, external for 1.16 and later
```
这个地方写`*`会不会造成安全隐患，暂时没想到，反正也是自己用，这样比较方便