# 响应区域
- 依据响应的header 如果ContentType 是 application/json 就默认开JSON高亮 否则就维持现状
- 响应区域的Headers tab 现在只有响应的，把请求的Headers 也渲染出来吧
- 响应区域的SSE请求，单个event点击后 弹出的JSON高亮的 窗口需要能调节高度。

# 左侧Request树
- 现在启动的时候会加载关闭前打开的Request，然后左侧Request树也需要对应展开和焦点定位到这个Request。


# 请求区域
- 发送按钮改成 发送的icon，然后去掉保存按钮，去掉代码按钮 代码里的复制curl已经在Request右键里有了，那么把复制为RawHTTP也加到右键的下拉功能里去
