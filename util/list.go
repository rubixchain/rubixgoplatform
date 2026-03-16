package util

func RemoveElementFromList[T comparable](list []T, elemToRemove T) []T {
    result := make([]T, 0, len(list))
    for _, elem := range list {
        if elem != elemToRemove {
            result = append(result, elem)
        }
    }
    return result
}

func SearchElementInList[T comparable](list []T, elemToSearch T) (int, bool) {
    for i, elem := range list {
        if elem == elemToSearch {
            return i, true
        }
    }

    return -1, false
}

func FindCommonElementsInList[T comparable](list1 []T, list2 []T) []T {
    result := make([]T, 0)
    lookup := make(map[T]struct{}, len(list2))

    for _, elem := range list2 {
        lookup[elem] = struct{}{}
    }

    for _, elem := range list1 {
        if _, found := lookup[elem]; found {
            result = append(result, elem)
        }
    }

    return result
}

func RemoveElementsFromList[T comparable](list []T, elemsToRemove []T) []T {
    result := make([]T, 0, len(list))
    lookup := make(map[T]struct{}, len(elemsToRemove))

    for _, elem := range elemsToRemove {
        lookup[elem] = struct{}{}
    }

    for _, elem := range list {
        if _, found := lookup[elem]; !found {
            result = append(result, elem)
        }
    }

    return result
}