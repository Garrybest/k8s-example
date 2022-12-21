package schedtest

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	"github.com/Garrybest/k8s-example/pkg/util"
)

type options struct {
	master     string
	kubeConfig string
	// ClusterAPIQPS is the QPS to use while talking with cluster kube-apiserver.
	clusterAPIQPS float32
	// ClusterAPIBurst is the burst to allow while talking with cluster kube-apiserver.
	clusterAPIBurst int
	namespace       string
}

func newOptions() *options {
	return &options{}
}

// addFlags adds flags of scheduler to the specified FlagSet
func (o *options) addFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.StringVar(&o.kubeConfig, "kubeconfig", o.kubeConfig, "Path to control plane kubeconfig file.")
	fs.StringVar(&o.master, "master", o.master, "The address of the member Kubernetes API server. Overrides any value in KubeConfig. Only required if out-of-cluster.")
	fs.Float32Var(&o.clusterAPIQPS, "kube-api-qps", 20.0, "QPS to use while talking with apiserver. Doesn't cover events and node heartbeat apis which rate limiting is controlled by a different set of flags.")
	fs.IntVar(&o.clusterAPIBurst, "kube-api-burst", 30, "Burst to use while talking with apiserver. Doesn't cover events and node heartbeat apis which rate limiting is controlled by a different set of flags.")
	fs.StringVar(&o.namespace, "namespace", metav1.NamespaceAll, "Namespace that take effect.")
}

func NewCommand(ctx context.Context) *cobra.Command {
	opts := newOptions()

	cmd := &cobra.Command{
		Use:  "schedule-test",
		Long: `TBD`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := run(ctx, opts); err != nil {
				return err
			}
			return nil
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
	}

	fss := cliflag.NamedFlagSets{}

	genericFlagSet := fss.FlagSet("generic")
	opts.addFlags(genericFlagSet)

	// Set klog flags
	logsFlagSet := fss.FlagSet("logs")

	// Since klog only accepts golang flag set, so introduce a shim here.
	flagSetShim := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(flagSetShim)
	logsFlagSet.AddGoFlagSet(flagSetShim)

	cmd.Flags().AddFlagSet(genericFlagSet)
	cmd.Flags().AddFlagSet(logsFlagSet)

	return cmd
}

func run(ctx context.Context, opts *options) error {
	restConfig, err := clientcmd.BuildConfigFromFlags(opts.master, opts.kubeConfig)
	if err != nil {
		return err
	}
	restConfig.QPS, restConfig.Burst = opts.clusterAPIQPS, opts.clusterAPIBurst

	kubeClient := kubernetes.NewForConfigOrDie(restConfig)
	informerFactory := informers.NewSharedInformerFactoryWithOptions(kubeClient, 0, informers.WithNamespace(opts.namespace))

	t := newTask(kubeClient, informerFactory, opts, ctx.Done())
	if err = t.Start(ctx); err != nil {
		klog.Errorf("task exits unexpectedly: %v", err)
		return err
	}
	return nil
}

type task struct {
	client          kubernetes.Interface
	informerFactory informers.SharedInformerFactory

	lister    corelister.PodLister
	namespace string
}

func (t *task) Start(ctx context.Context) error {
	stopCh := ctx.Done()
	klog.Infoln("Starting example task")
	defer klog.Infoln("Stopping example task")

	t.informerFactory.Start(stopCh)
	t.informerFactory.WaitForCacheSync(stopCh)

	// add your logic here
	duration := 5 * time.Second
	ticker := time.Tick(duration)

	for {
		select {
		case <-ticker:
			pods, err := t.lister.Pods(t.namespace).List(labels.Everything())
			if err != nil {
				return fmt.Errorf("failed to list pods: %v", err)
			}

			var (
				scheduled int
				avg       time.Duration
				m         time.Duration
			)

			for _, pod := range pods {
				_, cond := util.GetPodCondition(&pod.Status, corev1.PodScheduled)
				if cond != nil && cond.Status == corev1.ConditionTrue {
					scheduled++
					scheduleTime := cond.LastTransitionTime.Sub(pod.CreationTimestamp.Time)
					avg += scheduleTime
					if scheduleTime > m {
						m = scheduleTime
					}
				}
			}

			if len(pods) > 0 {
				avg /= time.Duration(len(pods))
			}

			klog.Infof("All: %d, Scheduled: %d, Unscheduled: %d, Max: %v, Avg: %v", len(pods), scheduled, len(pods)-scheduled, m, avg)

		case <-stopCh:
			break
		}
	}
}

func newTask(client kubernetes.Interface, factory informers.SharedInformerFactory, opts *options, done <-chan struct{}) *task {
	t := &task{
		client:          client,
		informerFactory: factory,

		lister:    factory.Core().V1().Pods().Lister(),
		namespace: opts.namespace,
	}

	// add your logic here
	return t
}
