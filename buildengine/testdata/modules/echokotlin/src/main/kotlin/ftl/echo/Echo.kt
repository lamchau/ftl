package ftl.echo2

import ftl.test.TestResponse
import xyz.block.ftl.Context
import xyz.block.ftl.Method
import xyz.block.ftl.Verb

data class EchoRequest(val name: String? = "anonymous")
data class EchoResponse(val message: String)

@Verb
fun echo(context: Context, req: EchoRequest): EchoResponse {
  return EchoResponse(message = "Hello, ${req.name}!")
}
