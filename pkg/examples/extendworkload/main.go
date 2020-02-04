package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/oam-dev/oam-go-sdk/apis/common"

	"github.com/oam-dev/oam-go-sdk/pkg/client/clientset/versioned"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/oam-go-sdk/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/oam-go-sdk/pkg/oam"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	myscheme = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	ctrl.SetLogger(zap.Logger(true))
}

type ApplicationConfiguration struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`

	//in our case, we use the same spec with applicationConfiguration
	Spec   v1alpha1.ApplicationConfigurationSpec   `json:"spec,omitempty"`
	Status v1alpha1.ApplicationConfigurationStatus `json:"status,omitempty"`
}

type ApplicationConfigurationList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []ApplicationConfiguration `json:"items"`
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ApplicationConfiguration) DeepCopyInto(out *ApplicationConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ApplicationConfiguration.
func (in *ApplicationConfiguration) DeepCopy() *ApplicationConfiguration {
	if in == nil {
		return nil
	}
	out := new(ApplicationConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ApplicationConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ApplicationConfigurationList) DeepCopyInto(out *ApplicationConfigurationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ApplicationConfiguration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ApplicationConfigurationList.
func (in *ApplicationConfigurationList) DeepCopy() *ApplicationConfigurationList {
	if in == nil {
		return nil
	}
	out := new(ApplicationConfigurationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ApplicationConfigurationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func main() {
	var metricsAddr string
	var newCrd bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&newCrd, "new-crd", false, "work as new crd or not")
	flag.Parse()
	fmt.Println("new CRD? ", newCrd)
	if newCrd {
		// GroupVersion is group version used to register these objects
		var SchemeGroupVersion = schema.GroupVersion{Group: "ros.alibaba.com", Version: "v1alpha1"}
		// schemeBuilder is used to add go types to the GroupVersionKind scheme
		var SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}
		//register object kind for our new
		SchemeBuilder.Register(&ApplicationConfiguration{}, &ApplicationConfigurationList{})
		// in fact, we could directly use below code
		// SchemeBuilder.Register(&v1alpha1.ApplicationConfiguration{}, &v1alpha1.ApplicationConfigurationList{})
		// AddToScheme adds the types in this group-version to the given scheme.
		SchemeBuilder.AddToScheme(myscheme)
	} else {
		_ = v1alpha1.AddToScheme(myscheme)
	}

	options := ctrl.Options{Scheme: myscheme, MetricsBindAddress: metricsAddr}
	// init
	oam.InitMgr(ctrl.GetConfigOrDie(), options)
	oamclient, err := versioned.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Fatal("create client err: ", err)
	}
	client, err := NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Fatal("create custom client err: ", err)
	}
	if newCrd {
		oam.RegisterObject(oam.SType("applicationConfiguration"), new(ApplicationConfiguration))
		oam.RegisterHandlers(oam.SType("applicationConfiguration"), &Handler{name: "app", oamclient: oamclient, client: client, newCrd: newCrd})
		err = oam.Run(oam.WithSpec(oam.SType("applicationConfiguration")))
		if err != nil {
			panic(err)
		}
	} else {
		// register workloadtpye & trait hooks and handlers
		oam.RegisterHandlers(oam.STypeApplicationConfiguration, &Handler{name: "app", oamclient: oamclient, client: client, newCrd: newCrd})
		// reconcilers must register manualy
		// cloudnativeapp/oam-runtime/pkg/oam as a pkg should not do os.Exit(), instead of
		// panic or returning Error could be better
		err = oam.Run(oam.WithApplicationConfiguration())
		if err != nil {
			panic(err)
		}
	}
}

type Handler struct {
	oamclient *versioned.Clientset
	client    *rest.RESTClient
	name      string
	newCrd    bool
}

func (s *Handler) HandleComponent(namespace string, comp v1alpha1.ComponentConfiguration) error {
	compIns, err := s.oamclient.CoreV1alpha1().ComponentSchematics(namespace).Get(comp.ComponentName, v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get component %s err %v", comp.ComponentName, err)
	}
	settings, err := common.ExtractParams(comp.ParameterValues, compIns.Spec.WorkloadSettings)
	if err != nil {
		return err
	}
	for k, v := range settings {
		fmt.Printf("%s: %s\n", k, v)
	}
	return nil
}

func (s *Handler) Handle(ctx *oam.ActionContext, comp runtime.Object, eType oam.EType) error {
	switch appConfig := comp.(type) {
	case *v1alpha1.ApplicationConfiguration:
		for _, comp := range appConfig.Spec.Components {
			if err := s.HandleComponent(appConfig.Namespace, comp); err != nil {
				return err
			}
		}
		appConfig.Status.Phase = "updated"
		if _, err := s.oamclient.CoreV1alpha1().ApplicationConfigurations(appConfig.Namespace).UpdateStatus(appConfig); err != nil {
			return err
		}
	case *ApplicationConfiguration:
		for _, comp := range appConfig.Spec.Components {
			if err := s.HandleComponent(appConfig.Namespace, comp); err != nil {
				return err
			}
		}
		appConfig.Status.Phase = "updated"
		result := &ApplicationConfiguration{}
		if err := s.client.Put().
			Namespace(appConfig.Namespace).
			Resource("applicationconfigurations").
			Name(appConfig.Name).
			SubResource("status").
			Body(appConfig).
			Do().
			Into(result); err != nil {
			return err
		}
	default:
		return errors.New("type mismatch")
	}
	return nil
}

func NewForConfig(c *rest.Config) (*rest.RESTClient, error) {
	config := *c
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: "ros.alibaba.com", Version: "v1alpha1"}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(myscheme)}
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (s *Handler) Id() string {
	return "Handler"
}
