//go:build !(wasi && wasm32)

package runtime

func MapMakeInt() uintptr {
	return mapMakeWithKeyKind(0)
}

func MapMakeString() uintptr {
	return mapMakeWithKeyKind(1)
}

func MapGetInt(hdr uintptr, key uintptr) (uintptr, bool) {
	if hdr == 0 {
		return 0, false
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	data := ReadPtr(hdr)
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	if slots != 0 && hashCap > 0 {
		keyHash := mapIntHashKey(key)
		idx, _, found := mapIntFindIndex(data, mlen, slots, hashCap, key, keyHash)
		if found {
			entryAddr := data + uintptr(idx*MapEntrySize)
			return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
		}
		return 0, false
	}
	idx := mapFindIntKey(data, mlen, key)
	if idx >= 0 {
		entryAddr := data + uintptr(idx*MapEntrySize)
		return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
	}
	return 0, false
}

func MapGetString(hdr uintptr, key uintptr) (uintptr, bool) {
	if hdr == 0 {
		return 0, false
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	data := ReadPtr(hdr)
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	hashes := ReadPtr(hdr + uintptr(mapHdrOffHashes))
	if slots != 0 && hashes != 0 && hashCap > 0 {
		keyHash := mapStringHashKey(key)
		idx, _, found := mapStringFindIndex(data, mlen, slots, hashes, hashCap, key, keyHash)
		if found {
			entryAddr := data + uintptr(idx*MapEntrySize)
			return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
		}
		return 0, false
	}
	idx := mapFindStringKey(data, mlen, key)
	if idx >= 0 {
		entryAddr := data + uintptr(idx*MapEntrySize)
		return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
	}
	return 0, false
}

func MapSetInt(hdr uintptr, key uintptr, value uintptr) uintptr {
	if hdr == 0 {
		hdr = MapMakeInt()
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	data := ReadPtr(hdr)
	mcap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	if slots != 0 && hashCap > 0 {
		keyHash := mapIntHashKey(key)
		idx, _, found := mapIntFindIndex(data, mlen, slots, hashCap, key, keyHash)
		if found {
			entryAddr := data + uintptr(idx*MapEntrySize)
			WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
			return hdr
		}
		if mlen >= mcap {
			oldData := data
			oldSlots := slots
			oldCap := mcap
			oldHashCap := hashCap
			newCap := mcap * 2
			if newCap < 8 {
				newCap = 8
			}
			newData := mapAllocEntryBlock(newCap)
			if mlen > 0 {
				Memcopy(newData, data, mlen*MapEntrySize)
			}
			data = newData
			mcap = newCap
			WritePtr(hdr, data)
			WritePtr(hdr+uintptr(SliceOffCap), uintptr(mcap))

			needHashCap := mapHashCapForEntries(mcap)
			if hashCap != needHashCap {
				hashCap = needHashCap
				mapInitHashSlots(hdr, hashCap)
			}
			mapIntRebuildHashSlots(hdr, data, mlen)
			slots = ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
			hashCap = int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
			mapFreeEntryBlock(oldData, oldCap)
			if oldSlots != 0 && oldHashCap != hashCap {
				mapFreePtrBlock(oldSlots, oldHashCap)
			}
		}
		entryAddr := data + uintptr(mlen*MapEntrySize)
		WritePtr(entryAddr, key)
		WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
		WritePtr(hdr+uintptr(SliceOffLen), uintptr(mlen+1))
		mapInsertIndex(slots, hashCap, mlen, keyHash)
		return hdr
	}
	idx := mapFindIntKey(data, mlen, key)
	if idx >= 0 {
		entryAddr := data + uintptr(idx*MapEntrySize)
		WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
		return hdr
	}
	if mlen >= mcap {
		oldData := data
		oldCap := mcap
		newCap := mcap * 2
		if newCap < 8 {
			newCap = 8
		}
		newData := mapAllocEntryBlock(newCap)
		if mlen > 0 {
			Memcopy(newData, data, mlen*MapEntrySize)
		}
		WritePtr(hdr, newData)
		WritePtr(hdr+uintptr(SliceOffCap), uintptr(newCap))
		data = newData
		mcap = newCap
		mapFreeEntryBlock(oldData, oldCap)
	}
	entryAddr := data + uintptr(mlen*MapEntrySize)
	WritePtr(entryAddr, key)
	WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
	newLen := mlen + 1
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(newLen))
	if newLen >= mapHashMinLen {
		mapIntInitHashState(hdr, mcap)
		mapIntRebuildHashSlots(hdr, data, newLen)
	}
	return hdr
}

func MapSetString(hdr uintptr, key uintptr, value uintptr) uintptr {
	if hdr == 0 {
		hdr = MapMakeString()
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	data := ReadPtr(hdr)
	mcap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	hashes := ReadPtr(hdr + uintptr(mapHdrOffHashes))
	if slots != 0 && hashes != 0 && hashCap > 0 {
		keyHash := mapStringHashKey(key)
		idx, _, found := mapStringFindIndex(data, mlen, slots, hashes, hashCap, key, keyHash)
		if found {
			entryAddr := data + uintptr(idx*MapEntrySize)
			WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
			return hdr
		}
		if mlen >= mcap {
			oldData := data
			oldHashes := hashes
			oldSlots := slots
			oldCap := mcap
			oldHashCap := hashCap
			newCap := mcap * 2
			if newCap < 8 {
				newCap = 8
			}
			newData, newHashes := mapStringAllocDataHashes(newCap)
			if mlen > 0 {
				Memcopy(newData, data, mlen*MapEntrySize)
				Memcopy(newHashes, hashes, mlen*PtrSize)
			}
			if mlen < newCap {
				Memzero(newHashes+uintptr(mlen*PtrSize), (newCap-mlen)*PtrSize)
			}
			data = newData
			hashes = newHashes
			mcap = newCap
			WritePtr(hdr, data)
			WritePtr(hdr+uintptr(SliceOffCap), uintptr(mcap))
			WritePtr(hdr+uintptr(mapHdrOffHashes), hashes)

			needHashCap := mapHashCapForEntries(mcap)
			if hashCap != needHashCap {
				hashCap = needHashCap
				mapInitHashSlots(hdr, hashCap)
			}
			mapStringRebuildHashSlots(hdr, data, mlen)
			slots = ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
			hashCap = int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
			if mapStringUsesCombinedBlock(oldData, oldHashes, oldCap) {
				mapFreeStringBlock(oldData, oldCap)
			} else {
				mapFreeEntryBlock(oldData, oldCap)
				mapFreePtrBlock(oldHashes, oldCap)
			}
			if oldSlots != 0 && oldHashCap != hashCap {
				mapFreePtrBlock(oldSlots, oldHashCap)
			}
		}
		entryAddr := data + uintptr(mlen*MapEntrySize)
		WritePtr(entryAddr, key)
		WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
		WritePtr(hashes+uintptr(mlen*PtrSize), keyHash)
		WritePtr(hdr+uintptr(SliceOffLen), uintptr(mlen+1))
		mapInsertIndex(slots, hashCap, mlen, keyHash)
		return hdr
	}
	idx := mapFindStringKey(data, mlen, key)
	if idx >= 0 {
		entryAddr := data + uintptr(idx*MapEntrySize)
		WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
		return hdr
	}
	if mlen >= mcap {
		oldData := data
		oldCap := mcap
		newCap := mcap * 2
		if newCap < 8 {
			newCap = 8
		}
		newData := mapAllocEntryBlock(newCap)
		if mlen > 0 {
			Memcopy(newData, data, mlen*MapEntrySize)
		}
		data = newData
		mcap = newCap
		WritePtr(hdr, data)
		WritePtr(hdr+uintptr(SliceOffCap), uintptr(mcap))
		mapFreeEntryBlock(oldData, oldCap)
	}
	entryAddr := data + uintptr(mlen*MapEntrySize)
	WritePtr(entryAddr, key)
	WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
	newLen := mlen + 1
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(newLen))
	if newLen >= mapHashMinLen {
		mapStringInitHashState(hdr, mcap, 0)
		mapStringRebuildHashSlots(hdr, data, newLen)
	}
	return hdr
}

func MapDeleteInt(hdr uintptr, key uintptr) {
	if hdr == 0 {
		return
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	data := ReadPtr(hdr)
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	if slots != 0 && hashCap > 0 {
		keyHash := mapIntHashKey(key)
		idx, _, found := mapIntFindIndex(data, mlen, slots, hashCap, key, keyHash)
		if !found {
			return
		}
		lastIdx := mlen - 1
		if idx < lastIdx {
			entryAddr := data + uintptr(idx*MapEntrySize)
			lastAddr := data + uintptr(lastIdx*MapEntrySize)
			WritePtr(entryAddr, ReadPtr(lastAddr))
			WritePtr(entryAddr+uintptr(MapEntryOffVal), ReadPtr(lastAddr+uintptr(MapEntryOffVal)))
		}
		WritePtr(hdr+uintptr(SliceOffLen), uintptr(lastIdx))
		mapIntRebuildHashSlots(hdr, data, lastIdx)
		return
	}
	i := mapFindIntKey(data, mlen, key)
	if i < 0 {
		return
	}
	lastIdx := mlen - 1
	entryAddr := data + uintptr(i*MapEntrySize)
	if i < lastIdx {
		lastAddr := data + uintptr(lastIdx*MapEntrySize)
		WritePtr(entryAddr, ReadPtr(lastAddr))
		WritePtr(entryAddr+uintptr(MapEntryOffVal), ReadPtr(lastAddr+uintptr(MapEntryOffVal)))
	}
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(lastIdx))
}

func MapDeleteString(hdr uintptr, key uintptr) {
	if hdr == 0 {
		return
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	data := ReadPtr(hdr)
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	hashes := ReadPtr(hdr + uintptr(mapHdrOffHashes))
	if slots != 0 && hashes != 0 && hashCap > 0 {
		keyHash := mapStringHashKey(key)
		idx, _, found := mapStringFindIndex(data, mlen, slots, hashes, hashCap, key, keyHash)
		if !found {
			return
		}
		lastIdx := mlen - 1
		if idx < lastIdx {
			entryAddr := data + uintptr(idx*MapEntrySize)
			lastAddr := data + uintptr(lastIdx*MapEntrySize)
			WritePtr(entryAddr, ReadPtr(lastAddr))
			WritePtr(entryAddr+uintptr(MapEntryOffVal), ReadPtr(lastAddr+uintptr(MapEntryOffVal)))
			WritePtr(hashes+uintptr(idx*PtrSize), ReadPtr(hashes+uintptr(lastIdx*PtrSize)))
		}
		WritePtr(hashes+uintptr(lastIdx*PtrSize), 0)
		WritePtr(hdr+uintptr(SliceOffLen), uintptr(lastIdx))
		mapStringRebuildHashSlots(hdr, data, lastIdx)
		return
	}
	i := mapFindStringKey(data, mlen, key)
	if i < 0 {
		return
	}
	lastIdx := mlen - 1
	entryAddr := data + uintptr(i*MapEntrySize)
	if i < lastIdx {
		lastAddr := data + uintptr(lastIdx*MapEntrySize)
		WritePtr(entryAddr, ReadPtr(lastAddr))
		WritePtr(entryAddr+uintptr(MapEntryOffVal), ReadPtr(lastAddr+uintptr(MapEntryOffVal)))
	}
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(lastIdx))
}
