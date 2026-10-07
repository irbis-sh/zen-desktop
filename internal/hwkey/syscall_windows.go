package hwkey

//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output zsyscall_windows.go syscall_windows.go

//nolint:gocritic
//sys nCryptOpenStorageProvider(phProvider *nCryptProvHandle, pszProviderName *uint16, dwFlags uint32) (ret uint32) = ncrypt.NCryptOpenStorageProvider
//sys nCryptCreatePersistedKey(hProvider nCryptProvHandle, phKey *nCryptKeyHandle, pszAlgId *uint16, pszKeyName *uint16, dwLegacyKeySpec uint32, dwFlags uint32) (ret uint32) = ncrypt.NCryptCreatePersistedKey
//sys nCryptOpenKey(hProvider nCryptProvHandle, phKey *nCryptKeyHandle, pszKeyName *uint16, dwLegacyKeySpec uint32, dwFlags uint32) (ret uint32) = ncrypt.NCryptOpenKey
//sys nCryptDeleteKey(hKey nCryptKeyHandle, dwFlags uint32) (ret uint32) = ncrypt.NCryptDeleteKey
//sys nCryptSetProperty(hObject nCryptKeyHandle, pszProperty *uint16, pbInput *byte, cbInput uint32, dwFlags uint32) (ret uint32) = ncrypt.NCryptSetProperty
//sys nCryptFinalizeKey(hKey nCryptKeyHandle, dwFlags uint32) (ret uint32) = ncrypt.NCryptFinalizeKey
//sys nCryptExportKey(hKey nCryptKeyHandle, hExportKey nCryptKeyHandle, pszBlobType *uint16, pParameterList *byte, pbOutput *byte, cbOutput uint32, pcbResult *uint32, dwFlags uint32) (ret uint32) = ncrypt.NCryptExportKey
//sys nCryptSignHash(hKey nCryptKeyHandle, pPaddingInfo *byte, pbHashValue *byte, cbHashValue uint32, pbSignature *byte, cbSignature uint32, pcbResult *uint32, dwFlags uint32) (ret uint32) = ncrypt.NCryptSignHash
//sys nCryptFreeObject(hObject uintptr) (ret uint32) = ncrypt.NCryptFreeObject
