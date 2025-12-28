from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class WaddleRequest(_message.Message):
    __slots__ = ()
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    CHECK_KEY_FIELD_NUMBER: _ClassVar[int]
    GET_VAL_FIELD_NUMBER: _ClassVar[int]
    GET_LEN_FIELD_NUMBER: _ClassVar[int]
    GET_LAST_FIELD_NUMBER: _ClassVar[int]
    ADD_VAL_FIELD_NUMBER: _ClassVar[int]
    SEARCH_GLOBAL_FIELD_NUMBER: _ClassVar[int]
    SEARCH_KEY_FIELD_NUMBER: _ClassVar[int]
    SNAPSHOT_FIELD_NUMBER: _ClassVar[int]
    UPDATE_VAL_FIELD_NUMBER: _ClassVar[int]
    GET_KEYS_FIELD_NUMBER: _ClassVar[int]
    GET_VAL_LIST_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    check_key: CheckKeyRequest
    get_val: GetValueByIndexRequest
    get_len: GetLengthRequest
    get_last: GetLastValueRequest
    add_val: AddValueRequest
    search_global: SearchGlobalRequest
    search_key: SearchOnKeyRequest
    snapshot: SnapshotRequest
    update_val: UpdateValueRequest
    get_keys: GetKeysRequest
    get_val_list: GetValueListRequest
    def __init__(self, request_id: _Optional[str] = ..., check_key: _Optional[_Union[CheckKeyRequest, _Mapping]] = ..., get_val: _Optional[_Union[GetValueByIndexRequest, _Mapping]] = ..., get_len: _Optional[_Union[GetLengthRequest, _Mapping]] = ..., get_last: _Optional[_Union[GetLastValueRequest, _Mapping]] = ..., add_val: _Optional[_Union[AddValueRequest, _Mapping]] = ..., search_global: _Optional[_Union[SearchGlobalRequest, _Mapping]] = ..., search_key: _Optional[_Union[SearchOnKeyRequest, _Mapping]] = ..., snapshot: _Optional[_Union[SnapshotRequest, _Mapping]] = ..., update_val: _Optional[_Union[UpdateValueRequest, _Mapping]] = ..., get_keys: _Optional[_Union[GetKeysRequest, _Mapping]] = ..., get_val_list: _Optional[_Union[GetValueListRequest, _Mapping]] = ...) -> None: ...

class WaddleResponse(_message.Message):
    __slots__ = ()
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ITEM_FIELD_NUMBER: _ClassVar[int]
    LENGTH_FIELD_NUMBER: _ClassVar[int]
    SEARCH_RESULTS_FIELD_NUMBER: _ClassVar[int]
    KEY_LIST_FIELD_NUMBER: _ClassVar[int]
    VALUE_LIST_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    success: bool
    error_message: str
    item: DataItem
    length: int
    search_results: SearchResult
    key_list: KeyList
    value_list: ValueList
    def __init__(self, request_id: _Optional[str] = ..., success: _Optional[bool] = ..., error_message: _Optional[str] = ..., item: _Optional[_Union[DataItem, _Mapping]] = ..., length: _Optional[int] = ..., search_results: _Optional[_Union[SearchResult, _Mapping]] = ..., key_list: _Optional[_Union[KeyList, _Mapping]] = ..., value_list: _Optional[_Union[ValueList, _Mapping]] = ...) -> None: ...

class DataItem(_message.Message):
    __slots__ = ()
    ID_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    id: int
    timestamp: int
    payload: bytes
    def __init__(self, id: _Optional[int] = ..., timestamp: _Optional[int] = ..., payload: _Optional[bytes] = ...) -> None: ...

class CheckKeyRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    key: str
    def __init__(self, key: _Optional[str] = ...) -> None: ...

class GetValueByIndexRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    INDEX_FIELD_NUMBER: _ClassVar[int]
    key: str
    index: int
    def __init__(self, key: _Optional[str] = ..., index: _Optional[int] = ...) -> None: ...

class GetLengthRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    key: str
    def __init__(self, key: _Optional[str] = ...) -> None: ...

class GetLastValueRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    key: str
    def __init__(self, key: _Optional[str] = ...) -> None: ...

class AddValueRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    ITEM_FIELD_NUMBER: _ClassVar[int]
    key: str
    item: DataItem
    def __init__(self, key: _Optional[str] = ..., item: _Optional[_Union[DataItem, _Mapping]] = ...) -> None: ...

class UpdateValueRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    INDEX_FIELD_NUMBER: _ClassVar[int]
    ITEM_FIELD_NUMBER: _ClassVar[int]
    key: str
    index: int
    item: DataItem
    def __init__(self, key: _Optional[str] = ..., index: _Optional[int] = ..., item: _Optional[_Union[DataItem, _Mapping]] = ...) -> None: ...

class SearchGlobalRequest(_message.Message):
    __slots__ = ()
    PATTERN_FIELD_NUMBER: _ClassVar[int]
    pattern: bytes
    def __init__(self, pattern: _Optional[bytes] = ...) -> None: ...

class SearchOnKeyRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    PATTERN_FIELD_NUMBER: _ClassVar[int]
    key: str
    pattern: bytes
    def __init__(self, key: _Optional[str] = ..., pattern: _Optional[bytes] = ...) -> None: ...

class SnapshotRequest(_message.Message):
    __slots__ = ()
    SNAPSHOT_NAME_FIELD_NUMBER: _ClassVar[int]
    snapshot_name: str
    def __init__(self, snapshot_name: _Optional[str] = ...) -> None: ...

class SearchResult(_message.Message):
    __slots__ = ()
    ITEMS_FIELD_NUMBER: _ClassVar[int]
    items: _containers.RepeatedCompositeFieldContainer[DataItem]
    def __init__(self, items: _Optional[_Iterable[_Union[DataItem, _Mapping]]] = ...) -> None: ...

class GetKeysRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class KeyList(_message.Message):
    __slots__ = ()
    KEYS_FIELD_NUMBER: _ClassVar[int]
    keys: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, keys: _Optional[_Iterable[str]] = ...) -> None: ...

class GetValueListRequest(_message.Message):
    __slots__ = ()
    KEY_FIELD_NUMBER: _ClassVar[int]
    key: str
    def __init__(self, key: _Optional[str] = ...) -> None: ...

class ValueList(_message.Message):
    __slots__ = ()
    ITEMS_FIELD_NUMBER: _ClassVar[int]
    items: _containers.RepeatedCompositeFieldContainer[DataItem]
    def __init__(self, items: _Optional[_Iterable[_Union[DataItem, _Mapping]]] = ...) -> None: ...
