

import 'package:dartz/dartz.dart';

import '../network/failure.dart';

abstract class BaseUseCase<Result, Data>{

  Future<Either<Failure, Result>> call(Data input);

}