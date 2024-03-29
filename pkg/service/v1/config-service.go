package v1

import (
	"context"
	"encoding/base64"
	"github.com/xanzy/go-gitlab"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"log"
	"math/rand"
	"strings"
	"fmt"
	"bytes"
	"io"

	v1 "bitbucket.software.geant.org/projects/NMAAS/repos/nmaas-janitor/pkg/api/v1"
	"github.com/johnaoss/htpasswd/apr1"
)

const (
	apiVersion = "v1"
	namespaceNotFound = "Namespace not found"
)

type configServiceServer struct {
	kubeAPI kubernetes.Interface
	gitAPI *gitlab.Client
}

type basicAuthServiceServer struct {
	kubeAPI kubernetes.Interface
}

type certManagerServiceServer struct {
	kubeAPI kubernetes.Interface
}

type readinessServiceServer struct {
	kubeAPI kubernetes.Interface
}

type informationServiceServer struct {
	kubeAPI kubernetes.Interface
}

type podServiceServer struct {
	kubeAPI kubernetes.Interface
}

type namespaceServiceServer struct {
	kubeAPI kubernetes.Interface
}

func NewConfigServiceServer(kubeAPI kubernetes.Interface, gitAPI *gitlab.Client) v1.ConfigServiceServer {
	return &configServiceServer{kubeAPI: kubeAPI, gitAPI: gitAPI}
}

func NewBasicAuthServiceServer(kubeAPI kubernetes.Interface) v1.BasicAuthServiceServer {
	return &basicAuthServiceServer{kubeAPI: kubeAPI}
}

func NewCertManagerServiceServer(kubeAPI kubernetes.Interface) v1.CertManagerServiceServer {
	return &certManagerServiceServer{kubeAPI: kubeAPI}
}

func NewReadinessServiceServer(kubeAPI kubernetes.Interface) v1.ReadinessServiceServer {
	return &readinessServiceServer{kubeAPI: kubeAPI}
}

func NewInformationServiceServer(kubeAPI kubernetes.Interface) v1.InformationServiceServer {
	return &informationServiceServer{kubeAPI: kubeAPI}
}

func NewPodServiceServer(kubeAPI kubernetes.Interface) v1.PodServiceServer {
	return &podServiceServer{kubeAPI: kubeAPI}
}

func NewNamespaceServiceServer(kubeAPI kubernetes.Interface) v1.NamespaceServiceServer {
	return &namespaceServiceServer{kubeAPI: kubeAPI}
}

func logLine(message string) {
    log.Printf(message)
}

func checkAPI(api string, current string) error {
	if len(api) > 0 && current != api {
		return status.Errorf(codes.Unimplemented,
			"unsupported API version: service implements API version '%s', but asked for '%s'", apiVersion, api)
	}
	return nil
}

//Prepare response
func prepareResponse(status v1.Status, message string) *v1.ServiceResponse {
	return &v1.ServiceResponse {
		Api: apiVersion,
		Status: status,
		Message: message,
	}
}

//Prepare info response
func prepareInfoResponse(status v1.Status, message string, info string) *v1.InfoServiceResponse {
	return &v1.InfoServiceResponse {
		Api: apiVersion,
		Status: status,
		Message: message,
		Info: info,
	}
}

//Prepare pod list response
func preparePodListResponse(status v1.Status, message string, pods []*v1.PodInfo) *v1.PodListResponse {
	return &v1.PodListResponse {
		Api: apiVersion,
		Status: status,
		Message: message,
		Pods: pods,
	}
}

//Prepare pod logs response
func preparePodLogsResponse(status v1.Status, message string, lines []string) *v1.PodLogsResponse {
	return &v1.PodLogsResponse {
		Api: apiVersion,
		Status: status,
		Message: message,
		Lines: lines,
	}
}

//Find proper project, given user namespace and instance uid
func (s *configServiceServer) FindGitlabProjectId(api *gitlab.Client, uid string, domain string) (int, error) {
	//Find exact group
	logLine(fmt.Sprintf("Searching for GitLab Group by domain %s", domain))
	groups, _, err := api.Groups.SearchGroup(domain)
	if len(groups) != 1 || err != nil {
		logLine(fmt.Sprintf("Found %d groups in domain %s", len(groups), domain))
		log.Print(err)
		return -1, status.Errorf(codes.NotFound, "Gitlab Group for given domain does not exist")
	}

    var projectName = "groups-" + domain + "/" + uid
    logLine(fmt.Sprintf("Using given project name %s to obtain project id", projectName))

    project, _, err := api.Projects.GetProject(projectName, &gitlab.GetProjectOptions{})
    if err != nil {
        log.Print(err)
        return -1, status.Errorf(codes.NotFound, "Gitlab Project for given uid does not exist")
    }

    return project.ID, nil
}

//Parse repository files into string:string map for configmap creator
func (s *configServiceServer) PrepareDataMapFromRepository(api *gitlab.Client, repoId int) (map[string]map[string]string, error) {

	var compiledMap = map[string]map[string]string{}

	//Processing files in root directory
	logLine("Processing files in root directory")

	rootTree, _, err := api.Repositories.ListTree(repoId, nil)
	if err != nil {
		log.Print(err)
	}

	directoryMap := make(map[string]string)

	//Start parsing
	for _, file := range rootTree {

		if file.Type != "blob" {
			continue
		}
		logLine(fmt.Sprintf("Processing new file from repository (name: %s, path: %s)", file.Name, file.Path))

		opt := &gitlab.GetRawFileOptions{Ref: gitlab.String("master")}
		fileContent, _, err := api.RepositoryFiles.GetRawFile(repoId, file.Path, opt)
		if err != nil {
			log.Print(err)
			return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
		}

		//assign retrieved binary data to newly created configmap
		directoryMap[file.Name] = string(fileContent)
	}

	compiledMap[""] = directoryMap

	//List files recursively
	opt := &gitlab.ListTreeOptions{Recursive: gitlab.Bool(true)}
	treeRec, _, err := api.Repositories.ListTree(repoId, opt)

	//List directories (apart from root)
	for _, directory := range treeRec {

		if directory.Type == "tree" {

			logLine(fmt.Sprintf("Processing new directory from repository (name: %s, path: %s)", directory.Name, directory.Path))

			opt := &gitlab.ListTreeOptions{Path: gitlab.String(directory.Path), Recursive: gitlab.Bool(true)}
			dirTree, _, err := api.Repositories.ListTree(repoId, opt)
			if err != nil {
				log.Print(err)
			}

			directoryMap := make(map[string]string)

			//Start parsing
			for _, file := range dirTree {

				if file.Type != "blob" {
					continue
				}

				logLine(fmt.Sprintf("Processing new file from repository (name: %s, path: %s)", file.Name, file.Path))

				opt := &gitlab.GetRawFileOptions{Ref: gitlab.String("master")}
				fileContent, _, err := api.RepositoryFiles.GetRawFile(repoId, file.Path, opt)
				if err != nil {
					log.Print(err)
					return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
				}

				//assign retrieved binary data to newly created configmap
				directoryMap[file.Name] = string(fileContent)
			}

			compiledMap[directory.Name] = directoryMap
		}
	}

	return compiledMap, nil
}

//Create new configmap
func (s *configServiceServer) CreateOrReplace(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	proj, err := s.FindGitlabProjectId(s.gitAPI, depl.Uid, depl.Domain)
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Cannot find corresponding GitLap project"), err
	}

	//check if given k8s namespace exists
	_, err = s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		ns := apiv1.Namespace{}
		ns.Name = depl.Namespace
		_, err = s.kubeAPI.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
		}
	}

	var repo = map[string]map[string]string{}

	repo, err = s.PrepareDataMapFromRepository(s.gitAPI, proj)
	if err != nil {
		logLine("Error occurred while retrieving content of the Git repository. Will not create any ConfigMap")
		return prepareResponse(v1.Status_FAILED, "Failed to create ConfigMap"), err
	}

	for directory, files := range repo {

		cm := apiv1.ConfigMap{}
		if len(directory) > 0 {
			cm.SetName(depl.Uid + "-" + directory)
		} else {
			cm.SetName(depl.Uid)
		}
		cm.SetNamespace(depl.Namespace)
		cm.Data = files

		//check if configmap already exists
		_, err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})

		if err != nil { //Not exists, we create new
			_, err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Create(ctx, &cm, metav1.CreateOptions{})
			if err != nil {
				return prepareResponse(v1.Status_FAILED, "Failed to create ConfigMap"), err
			}
		} else { //Already exists, we update it
			_, err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Update(ctx, &cm, metav1.UpdateOptions{})
			if err != nil {
				return prepareResponse(v1.Status_FAILED, "Error while updating existing ConfigMap!"), err
			}
		}
	}

	return prepareResponse(v1.Status_OK, "ConfigMap created/updated successfully"), nil
}

//Delete all config maps for instance
func (s *configServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	//retrieve list of configmaps in namespace
	configMaps, err := s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK, "Could not retrieve list of ConfigMaps in namespace"), nil
	}

    for _, configmap := range configMaps.Items {
		//check if configmap belongs to instance
		if configmap.Name == depl.Uid || strings.Contains(configmap.Name, depl.Uid + "-") {
			//delete configmap
			logLine(fmt.Sprintf("Deleting ConfigMap named %s", configmap.Name))
			err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Delete(ctx, configmap.Name, metav1.DeleteOptions{})
			if err != nil {
				logLine(fmt.Sprintf("Error occurred while deleting ConfigMap %s", configmap.Name))
			}
		}
	}

	return prepareResponse(v1.Status_OK, "ConfigMaps deleted successfully"), nil
}

func randomString(l int) string {
	bytes := make([]byte, l)
	for i := 0; i < l; i++ {
		bytes[i] = byte(65 + rand.Intn(90-65))
	}
	return string(bytes)
}

func aprHashCredentials(user string, password string) (string, error) {
	out, err := apr1.Hash(password, randomString(8))
	if err != nil {
		return "", status.Errorf(codes.Internal, "Failed to execute apr hashing")
	}
	return user + ":" + out, nil
}

func (s *basicAuthServiceServer) PrepareSecretDataFromCredentials(credentials *v1.Credentials) (map[string][]byte, error) {
	hash, err := aprHashCredentials(credentials.User, credentials.Password)
	if err != nil {
		return nil, err
	}

	resultMap := make(map[string][]byte)
	resultMap["auth"] = []byte(hash)

	return resultMap, nil
}

func (s *basicAuthServiceServer) PrepareSecretJsonFromCredentials(credentials *v1.Credentials) ([]byte, error) {
	hash, err := aprHashCredentials(credentials.User, credentials.Password)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to execute htpasswd executable")
	}

	result := []byte("{\"data\": {\"auth\": \"")
	result = append(result, base64.StdEncoding.EncodeToString([]byte(hash))...)
	result = append(result, "\"}}"...)

	return result, nil
}

func getAuthSecretName(uid string) string {
	return uid + "-auth"
}

func (s *basicAuthServiceServer) CreateOrReplace(ctx context.Context, req *v1.InstanceCredentialsRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Instance

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil{
		ns := apiv1.Namespace{}
		ns.Name = depl.Namespace
		_, err = s.kubeAPI.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
		}
	}

	secretName := getAuthSecretName(depl.Uid)

	_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	//Secret does not exist, we have to create it
	if err != nil {
		//create secret
		secret := apiv1.Secret{}
		secret.SetNamespace(depl.Namespace)
		secret.SetName(secretName)
		secret.Data, err = s.PrepareSecretDataFromCredentials(req.Credentials)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while preparing secret!"), err
		}

		//commit secret
		_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Create(ctx, &secret, metav1.CreateOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while creating secret!"), err
		}

		return prepareResponse(v1.Status_OK, "Secret created successfully"), nil
	} else {
		patch, err := s.PrepareSecretJsonFromCredentials(req.Credentials)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while parsing configuration data"), err
		}

		//patch secret
		_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Patch(ctx, secretName, types.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while patching secret!"), err
		}
		return prepareResponse(v1.Status_OK, "Secret updated successfully"), nil
	}
}

func (s *basicAuthServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	secretName := getAuthSecretName(depl.Uid)

	//check if secret exist
	_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK, "Secret does not exist"), nil
	}

	//delete secret
	err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while removing secret!"), err
	}
	return prepareResponse(v1.Status_OK, "Secret deleted successfully"), nil
}

func (s *certManagerServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	secretName := depl.Uid + "-tls"

	//check if secret exist
	_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK, "Secret does not exist"), nil
	}

	//delete secret
	err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while removing secret!"), err
	}
	return prepareResponse(v1.Status_OK, "Secret deleted successfully"), nil
}

func (s *readinessServiceServer) CheckIfReady(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment
	logLine(fmt.Sprintf("> Check if deployment:%s ready in namespace:%s", depl.Uid, depl.Namespace))

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	logLine("looking for deployment and checking its status")
	dep, err := s.kubeAPI.AppsV1().Deployments(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
	if err != nil {
		logLine("deployment not found, looking for statefulset and checking its status")
		sts, err2 := s.kubeAPI.AppsV1().StatefulSets(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
		if err2 != nil {
			logLine("statefulset not found as well")
			return prepareResponse(v1.Status_FAILED, "Neither Deployment nor StatefulSet found!"), err2
		} else {
			logLine("statefulset found, verifying status")
			if *sts.Spec.Replicas == sts.Status.ReadyReplicas {
		        logLine("ready")
				return prepareResponse(v1.Status_OK, "StatefulSet is ready"), nil
			}
			logLine("not yet ready")
			return prepareResponse(v1.Status_PENDING, "Waiting for statefulset"), nil
		}
	} else {
		logLine("deployment found, verifying status")
		if *dep.Spec.Replicas == dep.Status.ReadyReplicas {
		    logLine("ready")
			return prepareResponse(v1.Status_OK, "Deployment is ready"), nil
		}
		logLine("not yet ready")
		return prepareResponse(v1.Status_PENDING, "Waiting for deployment"), nil
	}

}

func (s *informationServiceServer) RetrieveServiceIp(ctx context.Context, req *v1.InstanceRequest) (*v1.InfoServiceResponse, error) {

	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment
	logLine(fmt.Sprintf("> Retrieving IP for service:%s in namespace:%s", depl.Uid, depl.Namespace))

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareInfoResponse(v1.Status_FAILED, namespaceNotFound, ""), err
	}

	app, err := s.kubeAPI.CoreV1().Services(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
	if err != nil {
	    logLine("Service not found")
		return prepareInfoResponse(v1.Status_FAILED, "Service not found!", ""), err
	}

	if len(app.Status.LoadBalancer.Ingress) > 0 {
		logLine(fmt.Sprintf("Found %d load balancer ingresse(s)", len(app.Status.LoadBalancer.Ingress)))

		ip := app.Status.LoadBalancer.Ingress[0].IP

		if ip != "" {
			logLine(fmt.Sprintf("Found IP address. Will return %s", ip))
			return prepareInfoResponse(v1.Status_OK, "", ip), err
		} else {
			logLine("IP address not found")
			return prepareInfoResponse(v1.Status_FAILED, "Ip not found!", ""), err
		}

	} else {
		logLine("No load balancer ingresses found")
		return prepareInfoResponse(v1.Status_FAILED, "Service ingress not found!", ""), err
	}
}

func (s *informationServiceServer) CheckServiceExists(ctx context.Context, req *v1.InstanceRequest) (*v1.InfoServiceResponse, error) {

	logLine("> Entered CheckServiceExists method")

	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareInfoResponse(v1.Status_FAILED, namespaceNotFound, ""), err
	}

	logLine(fmt.Sprintf("About to read service %s details from namespace %s", depl.Uid, depl.Namespace))

	ser, err := s.kubeAPI.CoreV1().Services(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
	if err != nil {
		return prepareInfoResponse(v1.Status_FAILED, "Service not found!", ""), err
	}
	return prepareInfoResponse(v1.Status_OK, "", ser.Name), err
}

func (s *podServiceServer) RetrievePodList(ctx context.Context, req *v1.InstanceRequest) (*v1.PodListResponse, error) {
    logLine("> Entered RetrievePodList method")
    // check if the API version requested by client is supported by server
    if err := checkAPI(req.Api, apiVersion); err != nil {
        return nil, err
    }

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return preparePodListResponse(v1.Status_FAILED, namespaceNotFound, nil), err
	}

    //collecting all pods from given namespace
	logLine(fmt.Sprintf("Collecting pods from namespace %s", depl.Namespace))
	allPods, err := s.kubeAPI.CoreV1().Pods(depl.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return preparePodListResponse(v1.Status_FAILED, "Issue with collecting pods", nil), err
	}

	matchingPods := make([]*v1.PodInfo, 0)

    //filtering only those pods that match the deployment name
 	logLine(fmt.Sprintf("Filtering pods by deployment %s", depl.Uid))
	for _, pod := range allPods.Items {
	    if (strings.HasPrefix(pod.Name, depl.Uid + "-")) {
	        newPod := &v1.PodInfo{Name: pod.Name, DisplayName: pod.Name, Containers: make([]string, 0)}
	        for container := range pod.Spec.Containers {
                newPod.Containers = append(newPod.Containers, pod.Spec.Containers[container].Name)
            }
	        matchingPods = append(matchingPods, newPod)
	    }
	}

    logLine(fmt.Sprintf("< Found %d matching pods", len(matchingPods)))
	return preparePodListResponse(v1.Status_OK, "", matchingPods), err
}

func (s *podServiceServer) RetrievePodLogs(ctx context.Context, req *v1.PodRequest) (*v1.PodLogsResponse, error) {
    logLine("> Entered RetrievePodLogs method")
    // check if the API version requested by client is supported by server
    if err := checkAPI(req.Api, apiVersion); err != nil {
        return nil, err
    }

    depl := req.Deployment
	pod := req.Pod

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return preparePodLogsResponse(v1.Status_FAILED, namespaceNotFound, nil), err
	}

    //check if given pod exists
    _, err = s.kubeAPI.CoreV1().Pods(depl.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
    if err != nil {
        return preparePodLogsResponse(v1.Status_FAILED, "Pod not found", nil), err
    }

    //collecting logs from given pod
    var opts apiv1.PodLogOptions
    if len(pod.Containers) == 0 {
        opts = apiv1.PodLogOptions{}
    } else {
        opts = apiv1.PodLogOptions{Container: pod.Containers[0]}
    }
	logLine(fmt.Sprintf("Collecting logs from pod/container %s/%s in namespace %s", pod.Name, opts.Container, depl.Namespace))

	logsRequest := s.kubeAPI.CoreV1().Pods(depl.Namespace).GetLogs(pod.Name, &opts)

    podLogs, err := logsRequest.Stream(ctx)
	if err != nil {
		return preparePodLogsResponse(v1.Status_FAILED, "Issue with opening stream with logs", nil), err
	}
    defer podLogs.Close()

    logBuffer := new(bytes.Buffer)
    _, err = io.Copy(logBuffer, podLogs)
	if err != nil {
		return preparePodLogsResponse(v1.Status_FAILED, "Issue with copying data from stream to string", nil), err
	}
    logs := logBuffer.String()

    logLine(fmt.Sprintf("< Returning %d characters", len(logs)))
	return preparePodLogsResponse(v1.Status_OK, "", []string{logs}), err
}

func (s *namespaceServiceServer) CreateNamespace(ctx context.Context, req *v1.NamespaceRequest) (*v1.ServiceResponse, error) {
    logLine("> Entered CreateNamespace method")
    // check if the API version requested by client is supported by server
    if err := checkAPI(req.Api, apiVersion); err != nil {
        return nil, err
    }
	logLine(fmt.Sprintf("Creating namespace %s with %d annotations", req.Namespace, len(req.Annotations)))

	ns := apiv1.Namespace{}
	ns.Name = req.Namespace

	labels := make(map[string]string)
	labels["name"] = req.Namespace
	ns.SetLabels(labels)

	annotations := make(map[string]string)
    for _, a := range req.Annotations {
        annotations[a.Key] = a.Value
    }
    ns.SetAnnotations(annotations)

	_, err := s.kubeAPI.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	return prepareResponse(v1.Status_OK, ""), nil
}