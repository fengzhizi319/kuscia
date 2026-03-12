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

package domainroute

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/secretflow/kuscia/pkg/common"
	"github.com/secretflow/kuscia/pkg/controllers"
	dv1 "github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"
	kusciaclientset "github.com/secretflow/kuscia/pkg/crd/clientset/versioned"
	kusciafake "github.com/secretflow/kuscia/pkg/crd/clientset/versioned/fake"
	"github.com/secretflow/kuscia/pkg/crd/clientset/versioned/scheme"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	"github.com/secretflow/kuscia/pkg/utils/nlog/zlogwriter"
	"github.com/secretflow/kuscia/pkg/utils/signals"
	"github.com/secretflow/kuscia/pkg/utils/tls"
)

func createCrtString(t *testing.T) string {
	rootDir := t.TempDir()
	caCertFile := filepath.Join(rootDir, "ca.crt")
	caKeyFile := filepath.Join(rootDir, "ca.key")
	assert.NoError(t, tls.CreateCAFile("testca", caCertFile, caKeyFile))
	f, err := os.Open(caCertFile)
	assert.NoError(t, err)
	testCrt, err := io.ReadAll(f)
	assert.NoError(t, err)
	return base64.StdEncoding.EncodeToString(testCrt)
}

func Test_controller_with_token(t *testing.T) {
	logger, _ := zlogwriter.New(nil)
	nlog.Setup(nlog.SetWriter(logger))
	chStop := make(chan struct{})
	kusciaClient := kusciafake.NewSimpleClientset()
	kubeClient := kubefake.NewSimpleClientset()
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "test"})
	ctx := signals.NewKusciaContextWithStopCh(chStop)
	ic := NewController(ctx, controllers.ControllerConfig{
		KubeClient:    kubeClient,
		KusciaClient:  kusciaClient,
		EventRecorder: eventRecorder,
	})
	alice := "alicersa"
	bob := "bobrsa"
	go func() {
		var err error
		certstr := createCrtString(t)
		alicegateway := initDomain(ctx, t, kusciaClient, alice, certstr)
		bobgateway := initDomain(ctx, t, kusciaClient, bob, certstr)
		time.Sleep(100 * time.Millisecond)

		srcdr := initDomainRoute(alice, bob, certstr, alicegateway)
		destdr := initDomainRoute(alice, bob, certstr, alicegateway)
		destdr.ObjectMeta.Namespace = bob
		destdr.Status.TokenStatus.RevisionToken.Token = "bobtestToken"
		destdr.Status.TokenStatus.RevisionToken.Revision = 1
		destdr.Status.TokenStatus.RevisionToken.EffectiveInstances = []string{bobgateway.Name}

		_, err = kusciaClient.KusciaV1alpha1().DomainRoutes(alice).Create(ctx, srcdr, metav1.CreateOptions{})
		assert.NoError(t, err)
		_, err = kusciaClient.KusciaV1alpha1().DomainRoutes(bob).Create(ctx, destdr, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Wait for initial token synchronization to complete, using retry mechanism instead of fixed time sleep
		assert.Eventually(t, func() bool {
			srcdr, err = kusciaClient.KusciaV1alpha1().DomainRoutes(alice).Get(context.Background(), srcdr.Name, metav1.GetOptions{})
			if err != nil {
				nlog.Errorf("Failed to get source DomainRoute: %v", err)
				return false
			}
			destdr, err = kusciaClient.KusciaV1alpha1().DomainRoutes(bob).Get(context.Background(), destdr.Name, metav1.GetOptions{})
			if err != nil {
				nlog.Errorf("Failed to get destination DomainRoute: %v", err)
				return false
			}

			// Check if destination token status meets expected conditions
			if len(destdr.Status.TokenStatus.Tokens) != 1 {
				nlog.Debugf("Destination token count doesn't match expected, expected 1, actual %d", len(destdr.Status.TokenStatus.Tokens))
				return false
			}
			if !destdr.Status.TokenStatus.RevisionToken.IsReady {
				nlog.Debug("Destination RevisionToken.IsReady is false")
				return false
			}
			if destdr.Status.TokenStatus.Tokens[0].Token != "bobtestToken" {
				nlog.Debugf("Destination token value doesn't match expected, expected 'bobtestToken', actual '%s'", destdr.Status.TokenStatus.Tokens[0].Token)
				return false
			}
			if !destdr.Status.TokenStatus.Tokens[0].IsReady {
				nlog.Debug("Destination Tokens[0].IsReady is false")
				return false
			}
			if len(srcdr.Status.TokenStatus.Tokens) != 0 {
				nlog.Debugf("Source token count doesn't match expected, expected 0, actual %d", len(srcdr.Status.TokenStatus.Tokens))
				return false
			}
			return true
		}, 5*time.Second, 100*time.Millisecond, "Timeout waiting for initial token synchronization to complete")
		srcdr.Status.TokenStatus.RevisionToken.IsReady = true
		_, err = kusciaClient.KusciaV1alpha1().DomainRoutes(alice).Update(context.Background(), srcdr, metav1.UpdateOptions{})
		assert.NoError(t, err)

		// Wait for token synchronization to complete, using retry mechanism instead of fixed time sleep
		assert.Eventually(t, func() bool {
			srcdr, err = kusciaClient.KusciaV1alpha1().DomainRoutes(alice).Get(context.Background(), srcdr.Name, metav1.GetOptions{})
			if err != nil {
				nlog.Errorf("Failed to get DomainRoute: %v", err)
				return false
			}
			nlog.Debug(srcdr.Status.TokenStatus.RevisionToken, srcdr.Status.TokenStatus.Tokens)

			// Check if token status meets expected conditions
			if len(srcdr.Status.TokenStatus.Tokens) != 1 {
				nlog.Debugf("Token count doesn't match expected, expected 1, actual %d", len(srcdr.Status.TokenStatus.Tokens))
				return false
			}
			if !srcdr.Status.TokenStatus.RevisionToken.IsReady {
				nlog.Debug("RevisionToken.IsReady is false")
				return false
			}
			if srcdr.Status.TokenStatus.Tokens[0].Token != "alicetestToken" {
				nlog.Debugf("Token value doesn't match expected, expected 'alicetestToken', actual '%s'", srcdr.Status.TokenStatus.Tokens[0].Token)
				return false
			}
			if !srcdr.Status.TokenStatus.Tokens[0].IsReady {
				nlog.Debug("Tokens[0].IsReady is false")
				return false
			}
			return true
		}, 5*time.Second, 100*time.Millisecond, "Timeout waiting for token synchronization to complete")

		time.Sleep(200 * time.Millisecond)
		close(chStop)
	}()
	ic.Run(4)
	ic.Stop()
}

func initDomainRoute(alice string, bob string, certstr string, alicegateway *dv1.Gateway) *dv1.DomainRoute {
	srcdr := &dv1.DomainRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      alice + "-" + bob,
			Namespace: alice,
			Labels: map[string]string{
				common.KusciaSourceKey:      alice,
				common.KusciaDestinationKey: bob,
			},
		},
		Spec: dv1.DomainRouteSpec{
			Source:             alice,
			Destination:        bob,
			AuthenticationType: dv1.DomainAuthenticationToken,
			TokenConfig: &dv1.TokenConfig{
				TokenGenMethod:       dv1.TokenGenMethodRSA,
				SourcePublicKey:      getPublickeyFromCert(certstr),
				DestinationPublicKey: getPublickeyFromCert(certstr),
			},
		},
		Status: dv1.DomainRouteStatus{
			TokenStatus: dv1.DomainRouteTokenStatus{
				RevisionInitializer: alicegateway.Name,
				RevisionToken: dv1.DomainRouteToken{
					Token:              "alicetestToken",
					EffectiveInstances: []string{alicegateway.Name},
					Revision:           2,
					RevisionTime:       metav1.Now(),
					ExpirationTime:     metav1.NewTime(metav1.Now().Add(time.Second * 300)),
				},
			},
		},
	}
	return srcdr
}

func initDomain(ctx context.Context, t *testing.T, kusciaClient kusciaclientset.Interface, domainId string, certstr string) *dv1.Gateway {
	domain := &dv1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name: domainId,
		},
		Spec: dv1.DomainSpec{
			Cert: certstr,
		},
	}
	_, err := kusciaClient.KusciaV1alpha1().Domains().Create(ctx, domain, metav1.CreateOptions{})
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	gateway := &dv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testgw" + domainId,
			Namespace: domainId,
			Labels: map[string]string{
				"auth": "test",
			},
		},
		Status: dv1.GatewayStatus{
			HeartbeatTime: metav1.Time{
				Time: time.Now(),
			},
		},
	}

	_, err = kusciaClient.KusciaV1alpha1().Gateways(domainId).Create(ctx, gateway, metav1.CreateOptions{})
	assert.NoError(t, err)
	return gateway
}

func Test_controller_add_label(t *testing.T) {
	logger, _ := zlogwriter.New(nil)
	nlog.Setup(nlog.SetWriter(logger))
	chStop := make(chan struct{})
	kusciaClient := kusciafake.NewSimpleClientset()
	kubeClient := kubefake.NewSimpleClientset()
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("default")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "test"})
	ctx := signals.NewKusciaContextWithStopCh(chStop)
	ic := NewController(ctx, controllers.ControllerConfig{
		KubeClient:    kubeClient,
		KusciaClient:  kusciaClient,
		EventRecorder: eventRecorder,
	})
	alice := "aliceaddlabel"
	bob := "bobaddlabel"
	charlie := "charlieaddlabel"
	go func() {
		time.Sleep(300 * time.Millisecond)
		testdr := &dv1.DomainRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      alice + "-" + bob,
				Namespace: alice,
			},
			Spec: dv1.DomainRouteSpec{
				Source:             alice,
				Destination:        bob,
				AuthenticationType: dv1.DomainAuthenticationToken,
				TokenConfig: &dv1.TokenConfig{
					TokenGenMethod: dv1.TokenGenMethodRSA,
				},
			},
			Status: dv1.DomainRouteStatus{},
		}
		testdr2 := &dv1.DomainRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      alice + "-" + charlie,
				Namespace: alice,
				Labels: map[string]string{
					"auth": "test",
				},
			},
			Spec: dv1.DomainRouteSpec{
				Source:             alice,
				Destination:        charlie,
				AuthenticationType: dv1.DomainAuthenticationToken,
				TokenConfig: &dv1.TokenConfig{
					TokenGenMethod: dv1.TokenGenMethodRSA,
				},
			},
			Status: dv1.DomainRouteStatus{},
		}
		nlog.Debug("create ", testdr.Name, " ", testdr2.Name)
		_, err := kusciaClient.KusciaV1alpha1().DomainRoutes(testdr.Namespace).Create(ctx, testdr, metav1.CreateOptions{})
		assert.NoError(t, err)
		_, err = kusciaClient.KusciaV1alpha1().DomainRoutes(testdr2.Namespace).Create(ctx, testdr2, metav1.CreateOptions{})
		assert.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
		close(chStop)
	}()
	ic.Run(4)
	ic.Stop()
	time.Sleep(100 * time.Millisecond)
	dr, err := kusciaClient.KusciaV1alpha1().DomainRoutes(alice).Get(ctx, alice+"-"+bob, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, alice, dr.Labels[common.KusciaSourceKey])
	assert.Equal(t, bob, dr.Labels[common.KusciaDestinationKey])
	dr2, err := kusciaClient.KusciaV1alpha1().DomainRoutes(alice).Get(ctx, alice+"-"+charlie, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, alice, dr2.Labels[common.KusciaSourceKey])
	assert.Equal(t, charlie, dr2.Labels[common.KusciaDestinationKey])
}

func Test_Name(t *testing.T) {
	c := controller{}
	assert.Equal(t, controllerName, c.Name())
}

func getPublickeyFromCert(certString string) string {
	certPem, _ := base64.StdEncoding.DecodeString(certString)
	certData, _ := pem.Decode(certPem)

	cert, _ := x509.ParseCertificate(certData.Bytes)

	rsaPub, _ := cert.PublicKey.(*rsa.PublicKey)

	block := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(rsaPub),
	}

	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(block))
}
