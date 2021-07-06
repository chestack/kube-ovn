# use docker to generate code
# useage: bash ./hack/update-codegen-neutron.sh

# set GOPROXY you like
GOPROXY="https://goproxy.cn"

PROJECT_PACKAGE=github.com/kubeovn/kube-ovn
docker run -it --rm \
    -v ${PWD}:/go/src/${PROJECT_PACKAGE}\
    -e PROJECT_PACKAGE=${PROJECT_PACKAGE} \
    -e CLIENT_GENERATOR_OUT=${PROJECT_PACKAGE}/pkg/neutron/client \
    -e APIS_ROOT=${PROJECT_PACKAGE}/pkg/neutron/apis \
    -e GROUPS_VERSION="neutron:v1" \
    -e GENERATION_TARGETS="deepcopy,client,informer,lister" \
    -e GOPROXY=${GOPROXY} \
    quay.io/slok/kube-code-generator:v1.19.2
