package apis

import "k8s.io/apimachinery/pkg/util/sets"

type RegionTypeKey struct {
	Region       string
	InstanceType string
}

type RegionalInstancePrice struct {
	InstanceTypePrices map[string]*InstanceTypePrice `json:"instanceTypePrices"`
}

// GPUSpec the struct of `aws_instance_gpu.json`
// The content source of the JSON data:
// https://github.com/vantage-sh/ec2instances.info/blob/ac77935005ba75c0b95f6dcb5b5308163794147e/scrape.py#L524
// We need to update the JSON manual if the AWS add new gpu instance.
type GPUSpec struct {
	GPUModel          string  `json:"gpu_model"`
	ComputeCapability float64 `json:"compute_capability"`
	GPUCount          int     `json:"gpu_count"`
	// AlibabaCloud may split the gpu like `NVIDIA T4/8` to 8 partition.
	Partition int `json:"partition,omitempty"`
	CUDACores int `json:"cuda_cores"`
	GPUMemory int `json:"gpu_memory"`
}

type InstanceTypePrice struct {
	InstanceTypeMetadata
	Zones                []string `json:"zones"`
	OnDemandPricePerHour float64  `json:"onDemandPricePerHour"`
	// AWSEC2Billing represents the cost of saving plan billing
	// key is {savings plan type}/{term length}/{payment option}
	AWSEC2Billing map[string]AWSEC2Billing `json:"awsEC2Billing,omitempty"`
	// SpotPricePerHour represents the smallest spot price per hour in different zones
	SpotPricePerHour map[string]float64 `json:"spotPricePerHour,omitempty"`
}

type InstanceInfo struct {
	InstanceTypeMetadata
	RegionsSet sets.Set[string] `json:"-"`
	Regions    []string         `json:"regions"`
	GPU        *GPUSpec         `json:"gpu,omitempty"`
}

type InstanceTypeMetadata struct {
	Arch              string  `json:"arch"`
	PhysicalProcessor string  `json:"physical_processor"`
	ClockSpeed        float64 `json:"clock_speed"`
	VCPU              float64 `json:"vcpu"`
	Memory            float64 `json:"memory"`
	GPU               float64 `json:"gpu"`
	GPUArchitecture   string  `json:"gpu_architecture,omitempty"`
}

type Instance struct {
	Name string `json:"name"`
	InstanceTypeMetadata
}

type AWSEC2Billing struct {
	Rate float64 `json:"rate"`
}

type AWSEC2SPPaymentOption string

const (
	AWSEC2SPPaymentOptionAllUpfront     AWSEC2SPPaymentOption = "all"
	AWSEC2SPPaymentOptionPartialUpfront AWSEC2SPPaymentOption = "partial"
	AWSEC2SPPaymentOptionNoUpfront      AWSEC2SPPaymentOption = "no"
)

func (r *RegionalInstancePrice) DeepCopy() *RegionalInstancePrice {
	d := &RegionalInstancePrice{
		InstanceTypePrices: make(map[string]*InstanceTypePrice),
	}
	for k, v := range r.InstanceTypePrices {
		d.InstanceTypePrices[k] = v.DeepCopy()
	}
	return d
}

func (i *InstanceTypePrice) DeepCopy() *InstanceTypePrice {
	d := &InstanceTypePrice{
		InstanceTypeMetadata: InstanceTypeMetadata{
			Arch:              i.Arch,
			PhysicalProcessor: i.PhysicalProcessor,
			ClockSpeed:        i.ClockSpeed,
			VCPU:              i.VCPU,
			Memory:            i.Memory,
			GPU:               i.GPU,
			GPUArchitecture:   i.GPUArchitecture,
		},
		Zones:                make([]string, len(i.Zones)),
		OnDemandPricePerHour: i.OnDemandPricePerHour,
		AWSEC2Billing:        make(map[string]AWSEC2Billing),
		SpotPricePerHour:     make(map[string]float64),
	}
	copy(d.Zones, i.Zones)
	for k, v := range i.AWSEC2Billing {
		d.AWSEC2Billing[k] = v
	}
	for k, v := range i.SpotPricePerHour {
		d.SpotPricePerHour[k] = v
	}
	return d
}

func (g *GPUSpec) DeepCopy() *GPUSpec {
	if g == nil {
		return nil
	}
	return &GPUSpec{
		GPUModel:          g.GPUModel,
		ComputeCapability: g.ComputeCapability,
		GPUCount:          g.GPUCount,
		Partition:         g.Partition,
		CUDACores:         g.CUDACores,
		GPUMemory:         g.GPUMemory,
	}
}
