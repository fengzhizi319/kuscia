// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modules

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/secretflow/kuscia/cmd/kuscia/utils"
	pkgcom "github.com/secretflow/kuscia/pkg/common"
	kusciaapisv1alpha1 "github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"
	"github.com/secretflow/kuscia/pkg/utils/common"
	"github.com/secretflow/kuscia/pkg/utils/datastore"
	"github.com/secretflow/kuscia/pkg/utils/kubeconfig"
	"github.com/secretflow/kuscia/pkg/utils/network"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	"github.com/secretflow/kuscia/pkg/utils/nlog/ljwriter"
	"github.com/secretflow/kuscia/pkg/utils/paths"
	"github.com/secretflow/kuscia/pkg/utils/process"
	"github.com/secretflow/kuscia/pkg/utils/supervisor"
	tlsutils "github.com/secretflow/kuscia/pkg/utils/tls"
)

type k3sModule struct {
	// rootDir 是 Kuscia 的运行根目录，日志、证书、配置、二进制等都从这里派生。
	rootDir           string
	// kubeconfigFile 是 K3s 启动后生成或使用的 kubeconfig 路径，后续 kubectl/client-go 都依赖它访问 apiserver。
	kubeconfigFile    string
	// bindAddress / listenPort 决定嵌入式 apiserver 的监听地址与端口。
	bindAddress       string
	listenPort        string
	// dataDir 是 K3s 数据目录，包含 etcd/WAL/证书等关键运行时文件。
	dataDir           string
	// datastoreEndpoint 可选地指向外部 MySQL/Postgres/etcd；为空时使用本地嵌入式存储。
	datastoreEndpoint string
	// clusterToken 用于 K3s 集群鉴权与节点加入；单机模式下也保持配置一致性。
	clusterToken      string
	// hostIP 会传递给 K3s 的 --node-ip，用于声明当前节点对外可见地址。
	hostIP            string
	// enableAudit 用于控制是否开启 apiserver audit log。
	enableAudit       bool
	// LogConfig 决定 K3s 独立日志的输出位置与轮转策略。
	LogConfig         nlog.LogConfig
	// conf 持有模块运行时上下文，后续初始化 CRD、KubeClient、Domain 等都要使用。
	conf              *ModuleRuntimeConfigs

	// readyCh/readyError 用于把“K3s 可用且 Kuscia 初始化完成”的结果异步通知给模块管理器。
	// 注意这里的 ready 不仅表示进程启动，还表示 CRD、资源、kubeconfig 等初始化链路已经完成。
	readyCh    chan struct{}
	readyError error
}

// kusciaConfig 汇总了生成 Kuscia 专用 kubeconfig 所需的文件路径。
// 这些文件包括 server CA、client CA、cluster role 及 kubeconfig 模板。
type kusciaConfig struct {
	serverCertFile         string
	clientKeyFile          string
	clientCertFile         string
	clusterRoleFile        string
	clusterRoleBindingFile string
	kubeConfigTmplFile     string
	kubeConfig             string
}

func NewK3s(i *ModuleRuntimeConfigs) (Module, error) {
	// 如果用户未显式配置 clusterToken，则使用 DomainID 的 md5 作为稳定默认值。
	// 这样既避免空 token，也保证同一 DomainID 的行为可预测。
	var clusterToken string
	clusterToken = i.Master.ClusterToken
	if clusterToken == "" {
		clusterToken = fmt.Sprintf("%x", md5.Sum([]byte(i.DomainID)))
	}
	hostIP, err := network.GetHostIP()
	if err != nil {
		nlog.Fatal(err)
	}
	// NewK3s 仅负责组装模块所需的静态参数，不执行任何启动动作。
	// 真正的 K3s 启动发生在 Run() 中。
	return &k3sModule{
		rootDir:           i.RootDir,
		kubeconfigFile:    i.KubeconfigFile,
		bindAddress:       "0.0.0.0",
		listenPort:        "6443",
		hostIP:            hostIP,
		dataDir:           filepath.Join(i.RootDir, k3sDataDirPrefix),
		enableAudit:       false,
		datastoreEndpoint: i.Master.DatastoreEndpoint,
		clusterToken:      clusterToken,
		LogConfig:         *i.LogConfig,
		conf:              i,
		readyCh:           make(chan struct{}),
	}, nil
}

func (s *k3sModule) Run(ctx context.Context) error {
	// 第一步：预检查 datastoreEndpoint，尽早暴露错误，避免 K3s 进程拉起后再失败。
	err := datastore.CheckDatastoreEndpoint(s.datastoreEndpoint)
	if err != nil {
		nlog.Errorf("k3s check datastore endpoint failed with: %s", err.Error())
		return err
	}

	args := []string{
		// server 表示以控制平面模式启动 K3s。
		"server",
		"-v=5",
		// -d/-o 分别指定数据目录与 kubeconfig 输出位置。
		"-d=" + s.dataDir,
		"-o=" + s.kubeconfigFile,
		// Kuscia 只复用 K3s 的控制平面能力，因此禁用 agent/scheduler 等不需要的组件。
		"--disable-agent",
		"--bind-address=" + s.bindAddress,
		"--https-listen-port=" + s.listenPort,
		"--node-ip=" + s.hostIP,
		"--disable-cloud-controller",
		"--disable-network-policy",
		"--disable-scheduler",
		// Kuscia 自己负责网络/网关/服务发现相关能力，因此关闭 flannel/coredns/servicelb 等默认组件。
		"--flannel-backend=none",
		"--disable=traefik",
		"--disable=coredns",
		"--disable=servicelb",
		"--disable=local-storage",
		"--disable=metrics-server",
		// 缩短 apiserver event TTL，降低事件对象长期堆积带来的存储压力。
		"--kube-apiserver-arg=event-ttl=10m",
	}
	// 非 root 用户下追加 rootless 参数，使嵌入式 K3s 尽量适配无特权环境。
	if !pkgcom.IsRootUser() {
		args = append(args, "--rootless")
	}
	// 如配置了外部 datastore，则显式交给 K3s 使用该后端。
	if s.datastoreEndpoint != "" {
		args = append(args, "--datastore-endpoint="+s.datastoreEndpoint)
	}
	// 统一注入 token，便于单机与多节点场景使用相同逻辑。
	if s.clusterToken != "" {
		args = append(args, "--token="+s.clusterToken)
	}
	// 如果开启 audit，则追加 apiserver audit log 相关参数。
	if s.enableAudit {
		args = append(args,
			"--kube-apiserver-arg=audit-log-path="+filepath.Join(s.rootDir, pkgcom.LogPrefix, "k3s-audit.log"),
			"--kube-apiserver-arg=audit-policy-file="+filepath.Join(s.rootDir, pkgcom.ConfPrefix, "k3s/k3s-audit-policy.yaml"),
			"--kube-apiserver-arg=audit-log-maxbackup=10",
			"--kube-apiserver-arg=audit-log-maxsize=300",
		)
	}

	sp := supervisor.NewSupervisor("k3s", nil, -1)
	s.LogConfig.LogPath = buildK3sLogPath(s.rootDir)
	lj, _ := ljwriter.New(&s.LogConfig)
	n := nlog.NewNLog(nlog.SetWriter(lj))

	// 这里异步执行“就绪检测 + Kuscia 环境初始化”。
	// 这样可以让 K3s 子进程启动与 ready 检测并发进行，并通过 readyCh 对外暴露最终结果。
	go func() {
		s.readyError = s.startCheckReady(ctx)
		if s.readyError == nil {
			// 只有当 /readyz 检查通过后，才继续注册 CRD、生成 kubeconfig、创建 Domain 等资源。
			s.readyError = s.initKusciaEnvAfterReady(ctx)
		}
		close(s.readyCh)
		nlog.Infof("close k3s ready chan")
	}()

	// supervisor 负责托管真实的 k3s 子进程：
	// 1. 统一进程生命周期；2. 传递日志；3. 在需要时配合 OOMScore 等策略。
	err = sp.Run(ctx, func(ctx context.Context) supervisor.Cmd {
		cmd := exec.Command(filepath.Join(s.rootDir, "bin/k3s"), args...)
		cmd.Stderr = n
		cmd.Stdout = n

		// 扩展 K3s 自动签发证书的有效期，减少频繁轮换。
		envs := os.Environ()
		envs = append(envs, "CATTLE_NEW_SIGNED_CERT_EXPIRATION_DAYS=3650")
		cmd.Env = envs
		return &ModuleCMD{
			cmd:   cmd,
			score: &k3sOOMScore,
		}
	})

	// 如果 K3s 主进程启动失败，则额外读取最后几行日志，帮助快速诊断。
	if err != nil {
		readK3sLog(s.LogConfig.LogPath, 10)
	}

	return err
}

func (s *k3sModule) WaitReady(ctx context.Context) error {
	// WaitReady 供模块管理器调用，用来阻塞等待“控制平面真正可用”。
	// 返回的不是单纯的进程状态，而是完整初始化链路的结果。
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.readyCh:
		nlog.Infof("k3s is ready now")
		return s.readyError
	}
}

func (s *k3sModule) Name() string {
	// 模块名用于模块管理器中的注册、依赖和日志标识。
	return "k3s"
}

func (s *k3sModule) startCheckReady(ctx context.Context) error {
	// 整体超时时间为 30 秒；每 1 秒探测一次 readyz。
	// 这样既避免忙等，也不会让启动阶段长时间无反馈。
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	tickerReady := time.NewTicker(time.Second)
	defer tickerReady.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tickerReady.C:
			// readyz() 同时校验：
			// 1. k3s 进程存在；
			// 2. server/client 证书已生成；
			// 3. https://127.0.0.1:<port>/readyz 返回 ok。
			if nil == s.readyz("https://127.0.0.1:"+s.listenPort) {
				return nil
			}
		case <-ticker.C:
			return fmt.Errorf("wait k3s ready timeout")
		}
	}
}

func (s *k3sModule) initKusciaEnvAfterReady(ctx context.Context) error {
	// 第二阶段初始化：K3s apiserver ready 后，继续补齐 Kuscia 业务运行所需的资源。
	// 顺序不能随意打乱，因为后面的步骤依赖前面的产物。

	// 1. 注册所有 CRD，让 apiserver 能识别 Kuscia 自定义资源。
	if err := applyCRD(s.conf); err != nil {
		nlog.Errorf("Init failed after k3s started with err: %s", err.Error())
		return err
	}
	if err := applyKusciaResources(s.conf); err != nil {
		nlog.Errorf("Init failed after k3s started with err: %s", err.Error())
		return err
	}
	// 3. 生成 Kuscia 自己使用的 kubeconfig，并下发必要 cluster role/binding。
	if err := genKusciaKubeConfig(s.conf); err != nil {
		nlog.Errorf("Init failed after k3s started with err: %s", err.Error())
		return err
	}

	// 4. 基于已经就绪的 kubeconfig/apiserver 初始化 client-go 客户端集合。
	clients, err := kubeconfig.CreateClientSetsFromKubeconfig(s.conf.KubeconfigFile, s.conf.ApiserverEndpoint)
	if err != nil {
		nlog.Errorf("Init failed after k3s started with err: %s", err.Error())
		return err
	}

	s.conf.Clients = clients

	// 5. 创建或校正当前 Domain 对象，确保 Domain 证书信息已经注册进控制平面。
	if err := createDefaultDomain(ctx, s.conf); err != nil {
		nlog.Errorf("Init failed after k3s started with err: %s", err.Error())
		return err
	}
	nlog.Infof("Register namespace %s succeed.", s.conf.DomainID)

	// 6. 创建跨域共享命名空间，供跨域协同资源使用。
	if err := createCrossNamespace(ctx, s.conf); err != nil {
		nlog.Errorf("Init failed after k3s started with err: %s", err.Error())
		return err
	}

	nlog.Infof("K3s init done")

	return nil

}

func (s *k3sModule) readyz(host string) error {
	// 第 1 层检查：确认 k3s 进程仍然存在。
	// 如果进程已退出，就没必要继续做证书或 HTTP 探测。
	if !process.CheckExists("k3s") {
		errMsg := "process [k3s] is not exists"
		nlog.Error(errMsg)
		return errors.New(errMsg)
	}

	// 第 2 层检查：确认 server/client 证书文件已生成。
	// readyz API 需要这些证书来建立双向 TLS 连接。
	serverCaFilePath := filepath.Join(s.dataDir, "server/tls/server-ca.crt")
	clientAdminCrtFilePath := filepath.Join(s.dataDir, "server/tls/client-admin.crt")
	clientAdminKeyFilePath := filepath.Join(s.dataDir, "server/tls/client-admin.key")

	if fileExistError := paths.CheckAllFileExist(serverCaFilePath, clientAdminCrtFilePath, clientAdminKeyFilePath); fileExistError != nil {
		err := fmt.Errorf("%s. waiting k3s service starting", fileExistError)
		nlog.Warn(err)
		return err
	}

	// 读取 server CA，用于验证 apiserver 证书。
	caCertFile, err := os.ReadFile(serverCaFilePath)
	if err != nil {
		nlog.Error(err)
		return err
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertFile) {
		msg := "caCertFile format error"
		nlog.Error(msg)
		return fmt.Errorf("%s", msg)
	}
	// 读取 admin client 证书/私钥，用于访问 K3s /readyz。
	certPEMBlock, err := os.ReadFile(clientAdminCrtFilePath)
	if err != nil {
		nlog.Error(err)
		return err
	}

	keyPEMBlock, err := os.ReadFile(clientAdminKeyFilePath)
	if err != nil {
		nlog.Error(err)
		return err
	}

	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		nlog.Error(err)
		return err
	}

	// 构造一个携带 client cert + root CA 的 HTTP 客户端，以双向 TLS 访问本地 apiserver。
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{cert},
		},
	}

	// 第 3 层检查：访问 /readyz 并要求返回正文严格等于 ok。
	cl := http.Client{}
	cl.Transport = tr
	req, err := http.NewRequest(http.MethodGet, host+"/readyz", nil)
	if err != nil {
		nlog.Errorf("NewRequest error:%s", err.Error())
		return err
	}
	resp, err := cl.Do(req)
	if err != nil {
		nlog.Errorf("Get ready err:%s", err.Error())
		return err
	}
	if resp == nil || resp.Body == nil {
		nlog.Error("resp must has body")
		return fmt.Errorf("resp must has body")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			nlog.Warnf("close readyz response body failed: %v", closeErr)
		}
	}()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		nlog.Error("ReadAll fail")
		return err
	}
	if string(respBytes) != "ok" {
		return errors.New("not ready")
	}
	return nil
}

func applyCRD(conf *ModuleRuntimeConfigs) error {
	// 扫描 crds/v1alpha1 目录下的所有 YAML 文件，并发执行 kubectl apply。
	// 这里并不关心具体 CRD 名称，只要文件在目录中就会被自动注册。
	dirPath := filepath.Join(conf.RootDir, "crds/v1alpha1")
	dirs, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}
	sw := sync.WaitGroup{}
	for _, dir := range dirs {
		if dir.IsDir() {
			continue
		}
		file := filepath.Join(dirPath, dir.Name())
		sw.Add(1)
		go func(f string) {
			// applyFile 内部失败会直接 fatal，说明这里假定 CRD 注册失败属于不可恢复错误。
			applyFile(conf, f)
			sw.Done()
		}(file)
	}
	sw.Wait()
	return nil
}

func genKusciaKubeConfig(conf *ModuleRuntimeConfigs) error {
	// 该函数完成两件事：
	// 1. 为 kuscia 生成独立客户端证书与 kubeconfig；
	// 2. 应用 cluster role / binding，确保该 kubeconfig 有足够权限。
	c := &kusciaConfig{
		serverCertFile:         filepath.Join(conf.RootDir, k3sDataDirPrefix, "server/tls/server-ca.crt"),
		clientKeyFile:          filepath.Join(conf.RootDir, k3sDataDirPrefix, "server/tls/client-ca.key"),
		clientCertFile:         filepath.Join(conf.RootDir, k3sDataDirPrefix, "server/tls/client-ca.crt"),
		clusterRoleFile:        filepath.Join(conf.RootDir, pkgcom.ConfPrefix, "kuscia-clusterrole.yaml"),
		clusterRoleBindingFile: filepath.Join(conf.RootDir, pkgcom.ConfPrefix, "kuscia-clusterrolebinding.yaml"),
		kubeConfigTmplFile:     filepath.Join(conf.RootDir, pkgcom.ConfPrefix, "kuscia.kubeconfig.tmpl"),
		kubeConfig:             conf.KusciaKubeConfig,
	}

	// 加载 K3s 生成的 server CA 与 client CA，后续用来签发 Kuscia 自己的客户端证书。
	serverCert, err := tlsutils.LoadCertFile(c.serverCertFile)
	if err != nil {
		return err
	}
	nlog.Info("load serverCertFile successfully")
	rootCert, rootKey, err := tlsutils.LoadX509EcKeyPair(c.clientCertFile, c.clientKeyFile)
	if err != nil {
		return err
	}
	nlog.Info("load rootCert, rootKey successfully")
	certTmpl := &x509.Certificate{
		// CommonName 为 kuscia，表明该证书代表 Kuscia 自己访问 apiserver。
		SerialNumber: big.NewInt(int64(uuid.New().ID())),
		Subject: pkix.Name{
			CommonName: "kuscia",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(10, 0, 0),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}
	var certOut, keyOut bytes.Buffer
	if err := tlsutils.GenerateX509KeyPair(rootCert, rootKey, certTmpl, &certOut, &keyOut); err != nil {
		return err
	}
	nlog.Info("generate certOut, keyOut successfully")
	// 使用模板渲染 kubeconfig，把 server CA、endpoint、client cert/key 都内嵌进去。
	s := struct {
		ServerCert string
		Endpoint   string
		KusciaCert string
		KusciaKey  string
	}{
		ServerCert: base64.StdEncoding.EncodeToString(serverCert),
		Endpoint:   conf.ApiserverEndpoint,
		KusciaCert: base64.StdEncoding.EncodeToString(certOut.Bytes()),
		KusciaKey:  base64.StdEncoding.EncodeToString(keyOut.Bytes()),
	}
	if err := common.RenderConfig(c.kubeConfigTmplFile, c.kubeConfig, s); err != nil {
		return err
	}
	nlog.Info("generate kuscia kubeconfig successfully")
	// 之后应用 RBAC 资源，确保这个 kubeconfig 背后的身份具备操作 Kuscia 资源所需权限。
	roleFiles := []string{
		c.clusterRoleFile,
		c.clusterRoleBindingFile,
	}
	sw := sync.WaitGroup{}
	for _, file := range roleFiles {
		sw.Add(1)
		go func(f string) {
			applyFile(conf, f)
			sw.Done()
		}(file)
	}
	sw.Wait()
	return nil
}

func applyKusciaResources(conf *ModuleRuntimeConfigs) error {
	// 这里应用的是 Kuscia 启动早期依赖的基础集群资源。
	// 当前主要是 domain-cluster-res.yaml，后续如有新增基础资源也可继续并入。
	resourceFiles := []string{
		filepath.Join(conf.RootDir, pkgcom.ConfPrefix, "domain-cluster-res.yaml"),
	}
	sw := sync.WaitGroup{}
	for _, file := range resourceFiles {
		sw.Add(1)
		go func(f string) {
			applyFile(conf, f)
			sw.Done()
		}(file)
	}
	sw.Wait()
	return nil
}

func applyFile(conf *ModuleRuntimeConfigs, file string) {
	// 统一通过内置 kubectl + kubeconfig 执行 apply。
	// 这样可以最大化复用 Kubernetes 原生 apply 语义，而不是手写 REST 调用。
	cmd := exec.Command(filepath.Join(conf.RootDir, "bin/kubectl"), "--kubeconfig", conf.KubeconfigFile, "apply", "-f", file)
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		// 这里直接 fatal，说明 Kuscia 把这些基础资源视为强依赖，失败后不再继续启动。
		nlog.Fatalf("apply %s err:%s", file, err.Error())
	}
	nlog.Infof("apply %s", file)
}

func createDefaultDomain(ctx context.Context, conf *ModuleRuntimeConfigs) error {
	// Domain 是 Kuscia 业务中的核心身份对象。
	// 启动时需要确保当前 Domain 已存在，且其中的 cert 与本地证书一致。
	nlog.Infof("try to regist namespace: %s", conf.DomainID)
	certRaw, err := os.ReadFile(conf.DomainCertFile)
	if err != nil {
		return err
	}

	certStr := base64.StdEncoding.EncodeToString(certRaw)
	_, err = conf.Clients.KusciaClient.KusciaV1alpha1().Domains().Create(ctx, &kusciaapisv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name: conf.DomainID,
		},
		Spec: kusciaapisv1alpha1.DomainSpec{
			Cert: certStr,
		}}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// 若已存在，则进一步检查证书是否一致；不一致时进行自动修正。
		dm, getErr := conf.Clients.KusciaClient.KusciaV1alpha1().Domains().Get(ctx, conf.DomainID, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		if dm.Spec.Cert != certStr {
			nlog.Warnf("domain %s cert is not match, will update", conf.DomainID)
			dm.Spec.Cert = certStr
			_, updateErr := conf.Clients.KusciaClient.KusciaV1alpha1().Domains().Update(ctx, dm, metav1.UpdateOptions{})
			return updateErr
		}
		return nil
	}

	return err
}

func createCrossNamespace(ctx context.Context, conf *ModuleRuntimeConfigs) error {
	// 创建系统级跨域命名空间，用于放置跨域协同所需的公共资源。
	// 如果已存在，则认为是幂等成功。
	if _, err := conf.Clients.KubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: pkgcom.KusciaCrossDomain,
		},
	}, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

// readK3sLog 在启动失败时读取 K3s 日志末尾内容，帮助问题定位。
func readK3sLog(k3sLogFilePath string, nLastLine int) {
	lines, readFileErr := utils.ReadLastNLinesAsString(k3sLogFilePath, nLastLine)
	if readFileErr != nil {
		nlog.Errorf("[k3s] read k3s file error: %v", readFileErr)
	} else {
		nlog.Errorf("[k3s] Log content: %s [k3s] Log end", lines)
	}
}

// buildK3sLogPath 统一计算 K3s 日志文件路径，保证与 Kuscia 根目录布局一致。
func buildK3sLogPath(rootDir string) string {
	return filepath.Join(rootDir, pkgcom.LogPrefix, "k3s.log")
}
