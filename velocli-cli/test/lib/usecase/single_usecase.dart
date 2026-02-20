import 'package:dartz/dartz.dart';

import '../network/failure.dart';

abstract class SingleUseCase<Result>{
  Future<Either<Failure, Result>> call();
}