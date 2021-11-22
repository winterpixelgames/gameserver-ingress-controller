package reconcilers

import (
	"fmt"
	"strconv"
	"strings"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	"github.com/Octops/gameserver-ingress-controller/pkg/gameserver"
	"github.com/pkg/errors"
	networkingv1 "k8s.io/api/networking/v1"
)

type IngressOption func(gs *agonesv1.GameServer, ingress *networkingv1.Ingress) error

func WithCustomAnnotations() IngressOption {
	return func(gs *agonesv1.GameServer, ingress *networkingv1.Ingress) error {
		annotations := ingress.Annotations
		for k, v := range gs.Annotations {
			if strings.HasPrefix(k, gameserver.OctopsAnnotationCustomPrefix) {
				custom := strings.TrimPrefix(k, gameserver.OctopsAnnotationCustomPrefix)
				if len(custom) == 0 {
					return errors.New("custom annotation does not contain a suffix")
				}
				annotations[custom] = v
			}
		}

		ingress.SetAnnotations(annotations)
		return nil
	}
}

func WithTLS(mode gameserver.IngressRoutingMode) IngressOption {
	return func(gs *agonesv1.GameServer, ingress *networkingv1.Ingress) error {
		errMsgInvalidAnnotation := func(mode, annotation string) error {
			return errors.Errorf("ingress routing mode %s requires the annotation %s to be set", mode, annotation)
		}

		tlsForDomain := func(gs *agonesv1.GameServer) (fqdn, secretName string, err error) {
			domain, ok := gameserver.HasAnnotation(gs, gameserver.OctopsAnnotationIngressDomain)
			if !ok {
				return "", "", errMsgInvalidAnnotation(mode.String(), gameserver.OctopsAnnotationIngressDomain)
			}

			return fmt.Sprintf("%s.%s", gs.Name, domain), fmt.Sprintf("%s-tls", gs.Name), nil
		}

		tlsForPath := func(gs *agonesv1.GameServer) (fqdn, secretName string, err error) {
			fqdn, ok := gameserver.HasAnnotation(gs, gameserver.OctopsAnnotationIngressFQDN)
			if !ok {
				return "", "", errMsgInvalidAnnotation(mode.String(), gameserver.OctopsAnnotationIngressFQDN)
			}

			return fqdn, fmt.Sprintf("%s-tls", gs.Name), nil
		}

		var host, secret string
		var err error

		switch mode {
		case gameserver.IngressRoutingModePath:
			host, secret, err = tlsForPath(gs)
		case gameserver.IngressRoutingModeDomain:
			fallthrough
		default:
			host, secret, err = tlsForDomain(gs)
		}

		if err != nil {
			return err
		}

		specificsecret, ok := gameserver.HasAnnotation(gs, gameserver.OctopsAnnotationSecretName)
		if ok {
			secret = specificsecret
		}

		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts: []string{
					host,
				},
				SecretName: secret,
			},
		}

		return nil
	}
}

func WithIngressRule(mode gameserver.IngressRoutingMode) IngressOption {
	return func(gs *agonesv1.GameServer, ingress *networkingv1.Ingress) error {
		errMsgInvalidAnnotation := func(mode, annotation, gsName string) error {
			return errors.Errorf("ingress routing mode %s requires the annotation %s to be present on %s, check your Fleet or GameServer manifest.", mode, annotation, gsName)
		}

		var host, path string

		switch mode {
		case gameserver.IngressRoutingModePath:
			fqdn, ok := gameserver.HasAnnotation(gs, gameserver.OctopsAnnotationIngressFQDN)
			if !ok {
				return errMsgInvalidAnnotation(mode.String(), gameserver.OctopsAnnotationIngressFQDN, gs.Name)
			}
			host, path = fqdn, "/"+gs.Name
		case gameserver.IngressRoutingModeDomain:
			fallthrough
		default:
			domain, ok := gameserver.HasAnnotation(gs, gameserver.OctopsAnnotationIngressDomain)
			if !ok {
				return errMsgInvalidAnnotation(mode.String(), gameserver.OctopsAnnotationIngressDomain, gs.Name)
			}
			host, path = fmt.Sprintf("%s.%s", gs.Name, domain), "/"
		}

		ingress.Spec.Rules = newIngressRule(host, path, gs.Name, gameserver.GetGameServerPort(gs).Port)
		return nil
	}
}

func WithTLSCertIssuer(issuerName string) IngressOption {
	return func(gs *agonesv1.GameServer, ingress *networkingv1.Ingress) error {
		terminate, ok := gameserver.HasAnnotation(gs, gameserver.OctopsAnnotationTerminateTLS)
		if !ok || len(terminate) == 0 {
			return nil
		}

		if terminateTLS, err := strconv.ParseBool(terminate); err != nil {
			return errors.Errorf("annotation %s for %s must be \"true\" or \"false\"", gameserver.OctopsAnnotationTerminateTLS, gs.Name)
		} else if terminateTLS == false {
			return nil
		}

		_, secok := gameserver.HasAnnotation(gs, gameserver.OctopsAnnotationSecretName)
		if secok {
			return nil
		}

		if len(issuerName) == 0 {
			return errors.Errorf("annotation %s for %s must be present, check your Fleet or GameServer manifest.", gameserver.OctopsAnnotationIssuerName, gs.Name)
		}

		ingress.Annotations[gameserver.CertManagerAnnotationIssuer] = issuerName
		return nil
	}
}

func newIngressRule(host, path, name string, port int32) []networkingv1.IngressRule {
	return []networkingv1.IngressRule{
		{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     path,
							PathType: &defaultPathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: name,
									Port: networkingv1.ServiceBackendPort{
										Number: port,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newIngressTLS(host, secretName string) []networkingv1.IngressTLS {
	return []networkingv1.IngressTLS{
		{
			Hosts: []string{
				strings.TrimSpace(host),
			},
			SecretName: fmt.Sprintf("%s-tls", strings.TrimSpace(secretName)),
		},
	}
}
