package deps

func GetRegionName(RegionID string) string {
	regionNames := map[string]string{
		"us-east-1":      "N. Virginia",
		"us-east-2":      "Ohio",
		"us-west-1":      "N. California",
		"us-west-2":      "Oregon",
		"ca-central-1":   "Central",
		"eu-central-1":   "Frankfurt",
		"eu-west-1":      "Ireland",
		"eu-west-2":      "London",
		"eu-west-3":      "Paris",
		"eu-north-1":     "Stockholm",
		"ap-northeast-1": "Tokyo",
		"ap-northeast-2": "Seoul",
		"ap-northeast-3": "Osaka",
		"ap-southeast-1": "Singapore",
		"ap-southeast-2": "Sydney",
		"ap-south-1":     "Mumbai",
		"sa-east-1":      "São Paulo",
	}
	if regionName, found := regionNames[RegionID]; found {
		return regionName
	}
	return RegionID
}

func AuthRegions() []string {
	regionsNames := []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"ca-central-1",
		"eu-central-1",
		"eu-west-1",
		"eu-west-2",
		"eu-west-3",
		"eu-north-1",
		"ap-northeast-1",
		"ap-northeast-2",
		"ap-northeast-3",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-south-1",
		"sa-east-1",
	}
	return regionsNames
}
