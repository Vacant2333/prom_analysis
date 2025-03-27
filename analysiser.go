package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/cloudpilot-ai/priceserver/pkg/apis"
	"github.com/cloudpilot-ai/priceserver/pkg/tools"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/klog"
)

type Analysiser struct {
	v1api v1.API

	priceClient tools.QueryClientInterface

	priceData *apis.RegionalInstancePrice
}

func NewAnalysiser(region string) (*Analysiser, error) {
	client, err := api.NewClient(api.Config{
		Address: "http://127.0.0.1:9090",
		Client: &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				Proxy:               http.ProxyFromEnvironment,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	})
	if err != nil {
		klog.Errorf("Failed to create Prometheus API client: %v", err)
		return nil, err
	}

	priceClient, err := tools.NewQueryClient("https://price.cloudpilot.ai", "aws", region)
	if err != nil {
		klog.Errorf("Failed to create priceserver client: %v", err)
		return nil, err
	}

	return &Analysiser{
		priceClient: priceClient,
		v1api:       v1.NewAPI(client),
		priceData:   priceClient.ListInstancesDetails(region),
	}, nil
}

func (a *Analysiser) GetTop3CheapestAndMostExpensiveInstanceTypes(ctx context.Context, region, zone string) ([]string, []string, error) {
	// 查询语句：当前时刻符合条件的所有实例
	query := fmt.Sprintf(`aws_spot_instance_price{region="%s", zone="%s"}`, region, zone)
	now := time.Now()
	result, warnings, err := a.v1api.Query(ctx, query, now)
	if err != nil {
		return nil, nil, fmt.Errorf("query failed: %w", err)
	}
	if len(warnings) > 0 {
		fmt.Println("Warnings:", warnings)
	}

	// 断言结果为 model.Vector
	vector, ok := result.(model.Vector)
	if !ok {
		return nil, nil, fmt.Errorf("result is not a vector")
	}
	if len(vector) == 0 {
		return nil, nil, fmt.Errorf("no data found")
	}

	// 按 spot 价格升序排序，排序后前 3 个为最便宜，最后 3 个为最贵
	// 注意：排序会改变 vector 的顺序
	sort.Slice(vector, func(i, j int) bool {
		return vector[i].Value < vector[j].Value
	})

	cheapestCount := 3
	expensiveCount := 3

	// 收集最便宜的实例（升序排序后，取前3个）
	cheapestList := make([]string, 0, cheapestCount)
	for i := 0; i < cheapestCount && i < len(vector); i++ {
		instanceType := string(vector[i].Metric["instance_type"])
		cheapestList = append(cheapestList, instanceType)
	}

	// 收集最贵的实例（升序排序后，取最后3个，需要倒序排列）
	expensiveList := make([]string, 0, expensiveCount)
	for i := len(vector) - 1; i >= 0 && len(expensiveList) < expensiveCount; i-- {
		instanceType := string(vector[i].Metric["instance_type"])
		expensiveList = append(expensiveList, instanceType)
	}

	return cheapestList, expensiveList, nil
}

// GetTop3InstanceTypes 分别查询 max_over_time 和 min_over_time，然后以 region+instance_type 为 key 存入 map，
// 最后计算相对比例 (max/min)，返回比例最大的 3 个 key
func (a *Analysiser) GetTop3InstanceTypes(ctx context.Context, region string) ([]string, error) {
	// 分别查询 max 和 min 值（直接在原始指标上使用范围向量）
	queryMax := fmt.Sprintf(`max_over_time((sum by(instance_type, region) (aws_spot_instance_price{region="%s"}))[1w:1m])`, region)
	queryMin := fmt.Sprintf(`min_over_time((sum by(instance_type, region) (aws_spot_instance_price{region="%s"}))[1w:1m])`, region)
	now := time.Now()
	maxResult, warnings, err := a.v1api.Query(ctx, queryMax, now)
	if err != nil {
		return nil, fmt.Errorf("query max failed: %w", err)
	}
	if len(warnings) > 0 {
		fmt.Println("Warnings (max):", warnings)
	}
	minResult, warnings, err := a.v1api.Query(ctx, queryMin, now)
	if err != nil {
		return nil, fmt.Errorf("query min failed: %w", err)
	}
	if len(warnings) > 0 {
		fmt.Println("Warnings (min):", warnings)
	}

	// 转换查询结果为 Vector 类型
	maxVector, ok := maxResult.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("max query result is not a vector")
	}
	minVector, ok := minResult.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("min query result is not a vector")
	}

	// 分别以 region+instance_type 为 key 存入 map
	maxMap := make(map[string]float64)
	for _, sample := range maxVector {
		instanceType := string(sample.Metric["instance_type"])
		// key 格式：region:instance_type
		key := fmt.Sprintf("%s:%s", region, instanceType)
		maxMap[key] = float64(sample.Value)
	}

	minMap := make(map[string]float64)
	for _, sample := range minVector {
		instanceType := string(sample.Metric["instance_type"])
		key := fmt.Sprintf("%s:%s", region, instanceType)
		minMap[key] = float64(sample.Value)
	}

	// 计算每个 key 的相对比例（max/min），要求 min>0
	ratioMap := make(map[string]float64)
	for key, maxVal := range maxMap {
		minVal, ok := minMap[key]
		if !ok || minVal <= 0 {
			continue
		}
		ratioMap[key] = maxVal / minVal
	}

	// 排序：先将 map 转换为 slice，按照 ratio 降序排序
	type kv struct {
		Key   string
		Ratio float64
	}
	var ratios []kv
	for k, v := range ratioMap {
		ratios = append(ratios, kv{Key: k, Ratio: v})
	}
	sort.Slice(ratios, func(i, j int) bool {
		return ratios[i].Ratio > ratios[j].Ratio
	})

	// 提取比例最大的 3 个 key
	topN := 3
	if len(ratios) < topN {
		topN = len(ratios)
	}
	top3 := make([]string, 0, topN)
	for i := 0; i < topN; i++ {
		top3 = append(top3, ratios[i].Key)
	}

	return top3, nil
}

func (a *Analysiser) GetTop3HighestSavingInstance(ctx context.Context, region string) ([][]string, error) {
	// 查询语句：只按 region 过滤，不再按 zone 筛选
	query := fmt.Sprintf(`aws_spot_instance_price{region="%s"}`, region)
	now := time.Now()
	result, warnings, err := a.v1api.Query(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	if len(warnings) > 0 {
		fmt.Println("Warnings:", warnings)
	}

	// 断言结果为 model.Vector
	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("result is not a vector")
	}
	if len(vector) == 0 {
		return nil, fmt.Errorf("no data found")
	}

	// 定义局部结构体用于保存每个样本的保存比例信息
	type SavingResult struct {
		InstanceType string
		Zone         string
		SavingRatio  float64
	}
	var results []SavingResult

	// 遍历所有样本，计算每个实例的节省比例
	for _, sample := range vector {
		spotPrice := float64(sample.Value)
		instanceType := string(sample.Metric["instance_type"])
		zone := string(sample.Metric["zone"])
		// 调用留空函数获取 OD 价格（请根据业务逻辑实现该函数）
		odPrice, err := a.GetOnDemandPrice(instanceType)
		if err != nil {
			fmt.Printf("failed to get OD price for %s: %v\n", instanceType, err)
			continue
		}
		if odPrice <= 0 {
			continue
		}
		savingRatio := (odPrice - spotPrice) / odPrice
		results = append(results, SavingResult{
			InstanceType: instanceType,
			Zone:         zone,
			SavingRatio:  savingRatio,
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no valid instance found")
	}

	// 按 savingRatio 降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].SavingRatio > results[j].SavingRatio
	})

	// 提取保存比例最高的 3 个结果，并构造 [][]string，每个内部切片包含 [instance_type, zone]
	topN := 3
	if len(results) < topN {
		topN = len(results)
	}
	out := make([][]string, 0, topN)
	for i := 0; i < topN; i++ {
		out = append(out, []string{results[i].InstanceType, results[i].Zone})
	}

	return out, nil
}

func (a *Analysiser) GetOnDemandPrice(instanceType string) (float64, error) {
	data, ok := a.priceData.InstanceTypePrices[instanceType]
	if !ok {
		return 0, fmt.Errorf("instance type %s not exist", instanceType)
	}
	return data.OnDemandPricePerHour, nil
}
