package daemon

import (
	"fmt"
	"github.com/alauda/kube-ovn/pkg/request"
	"github.com/alauda/kube-ovn/pkg/util"
	"github.com/emicklei/go-restful"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"net/http"
	"strings"
	"time"
)

type cniServerHandler struct {
	Config     *Configuration
	KubeClient kubernetes.Interface
}

func createCniServerHandler(config *Configuration) *cniServerHandler {
	csh := &cniServerHandler{KubeClient: config.KubeClient, Config: config}
	return csh
}

func (csh cniServerHandler) handleAdd(req *restful.Request, resp *restful.Response) {
	podRequest := request.PodRequest{}
	err := req.ReadEntity(&podRequest)
	if err != nil {
		klog.Errorf("parse add request failed %v", err)
		resp.WriteHeaderAndEntity(http.StatusBadRequest, err)
		return
	}
	klog.Infof("add port request %v", podRequest)
	var macAddr, ipAddr, cidr, gw, ingress, egress string
	for i := 0; i < 10; i++ {
		pod, err := csh.KubeClient.CoreV1().Pods(podRequest.PodNamespace).Get(podRequest.PodName, v1.GetOptions{})
		if err != nil {
			klog.Errorf("get pod %s/%s failed %v", podRequest.PodNamespace, podRequest.PodName, err)
			resp.WriteHeaderAndEntity(http.StatusInternalServerError, err)
			return
		}
		macAddr = pod.Annotations[util.MacAddressAnnotation]
		ipAddr = pod.Annotations[util.IpAddressAnnotation]
		cidr = pod.Annotations[util.CidrAnnotation]
		gw = pod.Annotations[util.GatewayAnnotation]
		ingress = pod.Annotations[util.IngressRateAnnotation]
		egress = pod.Annotations[util.EgressRateAnnotation]

		if macAddr == "" || ipAddr == "" || cidr == "" || gw == "" {
			// wait controller assign an address
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}

	if macAddr == "" || ipAddr == "" || cidr == "" || gw == "" {
		klog.Errorf("no available ip for pod %s/%s", podRequest.PodNamespace, podRequest.PodName)
		resp.WriteHeaderAndEntity(http.StatusInternalServerError, fmt.Sprintf("no available ip for pod %s/%s", podRequest.PodNamespace, podRequest.PodName))
		return
	}

	klog.Infof("create container mac %s, ip %s, cidr %s, gw %s", macAddr, ipAddr, cidr, gw)
	err = csh.configureNic(podRequest.PodName, podRequest.PodNamespace, podRequest.NetNs, podRequest.ContainerID, macAddr, ipAddr, gw, ingress, egress)
	if err != nil {
		klog.Errorf("configure nic failed %v", err)
		resp.WriteHeaderAndEntity(http.StatusInternalServerError, err)
		return
	}
	resp.WriteHeaderAndEntity(http.StatusOK, request.PodResponse{IpAddress: strings.Split(ipAddr, "/")[0], MacAddress: macAddr, CIDR: cidr, Gateway: gw})
}

func (csh cniServerHandler) handleDel(req *restful.Request, resp *restful.Response) {
	podRequest := request.PodRequest{}
	err := req.ReadEntity(&podRequest)
	if err != nil {
		klog.Errorf("parse del request failed %v", err)
		resp.WriteHeaderAndEntity(http.StatusBadRequest, err)
		return
	}
	klog.Infof("delete port request %v", podRequest)
	err = csh.deleteNic(podRequest.NetNs, podRequest.PodName, podRequest.PodNamespace, podRequest.ContainerID)
	if err != nil {
		klog.Errorf("del nic failed %v", err)
		resp.WriteHeaderAndEntity(http.StatusInternalServerError, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}
