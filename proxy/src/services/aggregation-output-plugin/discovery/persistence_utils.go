package discovery

import (
	"lunar/aggregation-plugin/common"
	sharedActions "lunar/shared-model/actions"
	sharedDiscovery "lunar/shared-model/discovery"
	"strings"

	"github.com/rs/zerolog/log"
)

const (
	endpointDelimiter = ":::"
)

func ConvertToPersisted(aggregations Agg) sharedDiscovery.Output {
	output := sharedDiscovery.Output{
		Interceptors: []sharedDiscovery.InterceptorOutput{},
		Endpoints:    map[string]sharedDiscovery.EndpointOutput{},
		Consumers:    map[string]map[string]sharedDiscovery.EndpointOutput{},
	}

	for endpoint, agg := range aggregations.Endpoints {
		key := dumpEndpoint(endpoint)
		output.Endpoints[key] = convertEndpointToPersisted(agg)
	}

	for consumer, endpoints := range aggregations.Consumers {
		output.Consumers[consumer] = map[string]sharedDiscovery.EndpointOutput{}
		for endpoint, agg := range endpoints {
			key := dumpEndpoint(endpoint)
			output.Consumers[consumer][key] = convertEndpointToPersisted(agg)
		}
	}

	for interceptor, agg := range aggregations.Interceptors {
		output.Interceptors = append(output.Interceptors, sharedDiscovery.InterceptorOutput{
			Type:    interceptor.Type,
			Version: interceptor.Version,
			LastTransactionDate: sharedActions.TimestampToStringFromInt64(
				agg.Timestamp),
		})
	}

	return output
}

func ConvertFromPersisted(output sharedDiscovery.Output) *Agg {
	aggregations := Agg{
		Interceptors: map[common.Interceptor]InterceptorAgg{},
		Endpoints:    map[common.Endpoint]EndpointAgg{},
		Consumers:    map[string]EndpointMapping{},
	}

	for key, endpoint := range output.Endpoints {
		parts := strings.Split(key, endpointDelimiter)
		minTime, err := sharedActions.TimestampFromStringToInt64(endpoint.MinTime)
		if err != nil {
			log.Error().Msgf("Error converting timestamp: %v", err)
			minTime = 0
		}
		maxTime, err := sharedActions.TimestampFromStringToInt64(endpoint.MaxTime)
		if err != nil {
			log.Error().Msgf("Error converting timestamp: %v", err)
			maxTime = 0
		}
		aggregations.Endpoints[common.Endpoint{
			Method: parts[0],
			URL:    parts[1],
		}] = convertEndpointFromPersisted(minTime, maxTime, endpoint)
	}

	for consumer, endpoints := range output.Consumers {
		aggregations.Consumers[consumer] = map[common.Endpoint]EndpointAgg{}
		for key, endpoint := range endpoints {
			parts := strings.Split(key, endpointDelimiter)
			if len(parts) < 2 {
				log.Error().Msgf("Invalid endpoint key: %v", key)
				continue
			}
			minTime, err := sharedActions.TimestampFromStringToInt64(endpoint.MinTime)
			if err != nil {
				log.Error().Msgf("Error converting timestamp: %v", err)
			}
			maxTime, err := sharedActions.TimestampFromStringToInt64(endpoint.MaxTime)
			if err != nil {
				log.Error().Msgf("Error converting timestamp: %v", err)
			}
			aggregations.Consumers[consumer][common.Endpoint{
				Method: parts[0],
				URL:    parts[1],
			}] = convertEndpointFromPersisted(minTime, maxTime, endpoint)
		}
	}

	for _, interceptor := range output.Interceptors {
		timestamp, err := sharedActions.TimestampFromStringToInt64(interceptor.LastTransactionDate)
		if err != nil {
			log.Error().Msgf("Error converting timestamp: %v", err)
			timestamp = 0
		}
		aggregations.Interceptors[common.Interceptor{
			Type:    interceptor.Type,
			Version: interceptor.Version,
		}] = InterceptorAgg{
			Timestamp: timestamp,
		}
	}

	return &aggregations
}

func dumpEndpoint(endpoint common.Endpoint) string {
	return strings.Join([]string{endpoint.Method, endpoint.URL}, endpointDelimiter)
}

func convertMapOfCountToInt(counts map[int]Count) map[int]int {
	result := make(map[int]int)
	for key, value := range counts {
		result[key] = int(value)
	}
	return result
}

func convertMapOfIntToCount(ints map[int]int) map[int]Count {
	result := make(map[int]Count)
	for key, value := range ints {
		result[key] = Count(value)
	}
	return result
}

func convertEndpointFromPersisted(
	minTime, maxTime int64,
	endpoint sharedDiscovery.EndpointOutput,
) EndpointAgg {
	return EndpointAgg{
		MinTime:         minTime,
		MaxTime:         maxTime,
		Count:           Count(endpoint.Count),
		StatusCodes:     convertMapOfIntToCount(endpoint.StatusCodes),
		AverageDuration: endpoint.AverageDuration,
	}
}

func convertEndpointToPersisted(agg EndpointAgg) sharedDiscovery.EndpointOutput {
	return sharedDiscovery.EndpointOutput{
		MinTime:         sharedActions.TimestampToStringFromInt64(agg.MinTime),
		MaxTime:         sharedActions.TimestampToStringFromInt64(agg.MaxTime),
		Count:           int(agg.Count),
		StatusCodes:     convertMapOfCountToInt(agg.StatusCodes),
		AverageDuration: agg.AverageDuration,
	}
}
