import httpx
import logging
import time
from typing import Any, Self


class RobustHTTPClient:
    """
    一个健壮的、具备自动重试和统一错误处理的HTTP客户端。

    该类的核心契约是：所有请求方法在指定策略下，
    要么成功返回一个`httpx.Response`对象，要么在所有尝试失败后返回`None`。
    它自身绝不向上抛出网络相关的异常，从而简化调用方的代码。
    """

    def __init__(self, retries: int = 3, delay: float = 2.0, timeout: float = 10.0):
        """
        初始化客户端。

        Args:
            retries (int): 单个请求的最大尝试次数。
            delay (float): 每次重试之间的等待时间（秒）。
            timeout (float): 每个HTTP请求的超时时间（秒）。
        """
        if retries < 1:
            raise ValueError("Retries must be at least 1")

        self.retries = retries
        self.delay = delay
        self.timeout = timeout
        self.logger = logging.getLogger(self.__class__.__name__)
        self.logger.info(
            f"客户端已初始化: Retries={self.retries}, Delay={self.delay}s, Timeout={self.timeout}s"
        )

    def _make_request(self, method: str, url: str, **kwargs: Any) -> dict[Any,Any] | None:
        """
        执行请求的核心私有方法，包含重试逻辑。
        """
        last_exception: Exception | None = None

        # 使用 httpx.Client 来管理连接池
        with httpx.Client(timeout=self.timeout) as client:
            for attempt in range(self.retries):
                try:
                    self.logger.debug(f"Attempt {attempt + 1}/{self.retries} for {method} {url}")

                    response = client.request(method, url, **kwargs)

                    # 检查4xx/5xx错误，如果存在则触发重试
                    response.raise_for_status()

                    self.logger.debug(f"Request successful for {url} with status {response.status_code}")
                    return response.json()

                except (httpx.RequestError, httpx.HTTPStatusError) as e:
                    last_exception = e
                    self.logger.warning(
                        f"Attempt {attempt + 1}/{self.retries} for {method} {url} failed. Error: {e}"
                    )
                    if attempt < self.retries - 1:
                        time.sleep(self.delay)

        self.logger.error(
            f"All {self.retries} attempts for {method} {url} failed. Last error: {last_exception}"
        )
        return None

    def get(
            self,
            url: str,
            *,
            headers: dict[str, str] | None = None,
            params: dict[str, Any] | None = None
    ) -> httpx.Response | None:
        """
        发送GET请求。

        Args:
            url (str): 目标URL。
            headers (dict | None): 请求头。
            params (dict | None): URL查询参数。

        Returns:
            httpx.Response | None: 成功则返回响应对象，否则返回None。
        """
        return self._make_request("GET", url, headers=headers, params=params)

    def post(
            self,
            url: str,
            *,
            headers: dict[str, str] | None = None,
            params: dict[str, Any] | None = None,
            data: dict[str, Any] | None = None,
            json: Any | None = None
    ) -> httpx.Response | None:
        """
        发送POST请求。

        Args:
            url (str): 目标URL。
            headers (dict | None): 请求头。
            params (dict | None): URL查询参数。
            data (dict | None): 表单数据。
            json (Any | None): JSON格式的数据。

        Returns:
            httpx.Response | None: 成功则返回响应对象，否则返回None。
        """
        return self._make_request("POST", url, headers=headers, params=params, data=data, json=json)