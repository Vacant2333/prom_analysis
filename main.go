package main

import (
	"context"
	"time"

	"k8s.io/klog/v2"
)

/*
需要的数据：
- 过去一周价格波动最显著的前3个实例
- 价格最高和价格最低的实例
- 相比OD，节省最多的实例
*/

var (
	retryAttempts = 20
	retrySleep    = 10 * time.Second
)

func main() {
	ctx := context.Background()
	region := "us-east-2"
	zone := "use2-az1"

	analysiser, err := NewAnalysiser(region)
	if err != nil {
		klog.Fatalf("Failed to create Analysiser: %v", err)
	}

	var top3 []string
	if err := retry(retryAttempts, retrySleep, func() error {
		var err error
		top3, err = analysiser.GetTop3InstanceTypes(ctx, region)
		return err
	}); err != nil {
		klog.Fatalf("Failed to get top3 instance types: %v", err)
	}

	var cheapest, mostExpensive []string
	if err := retry(retryAttempts, retrySleep, func() error {
		cheapest, mostExpensive, err = analysiser.GetTop3CheapestAndMostExpensiveInstanceTypes(ctx, region, zone)
		return err
	}); err != nil {
		klog.Fatalf("Failed to get cheapest and most expensive: %v", err)
	}

	var highestSaving [][]string
	if err := retry(retryAttempts, retrySleep, func() error {
		highestSaving, err = analysiser.GetTop3HighestSavingInstance(ctx, region)
		return err
	}); err != nil {
		klog.Fatalf("Failed to get highest saving instance: %v", err)
	}

	klog.Infof("Query region zone: %s %s", region, zone)
	klog.Infof("1. Top3 instance types: %v", top3)
	klog.Infof("2. Cheapest and most expensive on zone %s: \n"+
		"Cheapest: %v \n"+
		"Most Expensive: %v", zone, cheapest, mostExpensive)
	klog.Infof("3. Highest saving instance types: %v", highestSaving)
}

func retry(attempts int, sleep time.Duration, f func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = f(); err == nil {
			return nil
		}
		time.Sleep(sleep)
	}
	return err
}
