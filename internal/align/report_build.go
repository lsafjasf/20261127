package align

import "sort"

func buildEventInfos(idx *indexedSession) []EventInfo {
	result := make([]EventInfo, 0, len(idx.session.Events))
	for _, input := range idx.session.Events {
		state := idx.events[input.ID]
		info := EventInfo{
			ID: input.ID, Kind: input.Kind, Source: state.source,
			Confidence: rounded(state.confidence, 6),
		}
		if state.master == nil {
			info.Source = "unresolved"
			info.Confidence = 0
		} else {
			value := *state.master
			info.MasterTime = &value
		}
		for device := range state.observedBy {
			info.ObservationOf = append(info.ObservationOf, device)
		}
		sort.Strings(info.ObservationOf)
		result = append(result, info)
	}
	return result
}

func buildDeviceInfos(idx *indexedSession, states []*segmentState) []DeviceInfo {
	bySegment := make(map[string]*segmentState, len(states))
	for _, state := range states {
		bySegment[state.segment.ID] = state
	}
	result := make([]DeviceInfo, 0, len(idx.session.Devices))
	for _, device := range idx.session.Devices {
		info := DeviceInfo{
			ID: device.ID, Role: device.Role,
			Master: device.ID == idx.masterID,
		}
		if info.Role == "" && info.Master {
			info.Role = "master"
		}
		for _, segment := range device.Segments {
			info.Segments = append(info.Segments,
				buildSegmentReport(bySegment[segment.ID], idx))
		}
		result = append(result, info)
	}
	return result
}
